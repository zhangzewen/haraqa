package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/haraqa/haraqa/protocol"
	"github.com/haraqa/haraqa/server/queue"
)

type Option func(*Server) error

func WithQueue(q queue.Queue) Option {
	return func(s *Server) error {
		s.q = q
		return nil
	}
}

func WithDirs(dirs ...string) Option {
	return func(s *Server) error {
		s.dirs = dirs
		return nil
	}
}

func NewServer(options ...Option) (*Server, error) {
	s := &Server{
		Router: mux.NewRouter(),
	}

	for _, option := range options {
		if err := option(s); err != nil {
			return nil, err
		}
	}

	if len(s.dirs) == 0 {
		s.dirs = []string{".haraqa"}
	}
	if s.q == nil {
		// default queue
		var err error
		s.q, err = queue.NewFileQueue(s.dirs...)
		if err != nil {
			return nil, err
		}
	}
	if s.DefaultBatchSize == 0 {
		s.DefaultBatchSize = -1
	}

	s.Router.PathPrefix("/raw/").Handler(http.StripPrefix("/raw/", http.FileServer(http.Dir(s.q.RootDir()))))
	r := s.Router.PathPrefix("/topics").Subrouter()
	r.Path("/").Methods("GET").Handler(s.HandleGetAllTopics())
	r.Path("/{topic}").Methods("PUT").Handler(s.HandleCreateTopic())
	r.Path("/{topic}").Methods("PATCH").Handler(s.HandleModifyTopic())
	r.Path("/{topic}").Methods("DELETE").Handler(s.HandleDeleteTopic())
	r.Path("/{topic}").Methods("GET").Handler(s.HandleInspectTopic())
	r.Path("/{topic}").Methods("POST").Handler(s.HandleProduce())
	r.Path("/{topic}/{id}").Methods("GET").Handler(s.HandleConsume())
	//s.Router.Use(mux.CORSMethodMiddleware(s.Router))

	return s, nil
}

type Server struct {
	*mux.Router
	q queue.Queue

	DefaultBatchSize int64
	dirs             []string
}

func (s *Server) Close() error {
	return s.q.Close()
}

func (s *Server) HandleGetAllTopics() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		topics, err := s.q.ListTopics()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		var response []byte
		switch r.Header.Get("accept") {
		case "application/json":
			response, _ = json.Marshal(map[string][]string{
				"topics": topics,
			})
		default:
			response = []byte(strings.Join(topics, ","))
		}

		w.Write(response)
	}
}

func (s *Server) HandleCreateTopic() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body.Close()
		}

		topic := mux.Vars(r)["topic"]
		err := s.q.CreateTopic(topic)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}
}

func (s *Server) HandleModifyTopic() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body.Close()
		}

		topic := mux.Vars(r)["topic"]
		_, err := s.q.TruncateTopic(topic, 0)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
}

func (s *Server) HandleDeleteTopic() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body.Close()
		}

		topic := mux.Vars(r)["topic"]
		err := s.q.DeleteTopic(topic)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
}

func (s *Server) HandleInspectTopic() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body.Close()
		}

		topic := mux.Vars(r)["topic"]
		_, err := s.q.InspectTopic(topic)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
}

func (s *Server) HandleProduce() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)

		sizes, err := protocol.ReadSizes(r.Header)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		err = s.q.Produce(vars["topic"], sizes, r.Body)
		r.Body.Close()
		if err != nil {
			panic(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
}

func (s *Server) HandleConsume() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		topic := vars["topic"]
		id, err := strconv.ParseInt(vars["id"], 10, 64)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var n int64
		batchheader := r.Header.Get("X-BATCH-SIZE")
		if batchheader == "" {
			n = s.DefaultBatchSize
		} else {
			n, err = strconv.ParseInt(batchheader, 10, 64)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}

		info, err := s.q.Consume(topic, id, n)
		if err != nil {
			if os.IsNotExist(errors.Unwrap(err)) {
				w.WriteHeader(http.StatusNotFound)
			} else {
				w.WriteHeader(http.StatusInternalServerError)
			}
			return
		}
		if closer, ok := info.File.(io.Closer); ok {
			defer closer.Close()
		}
		if !info.Exists {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		w.Header().Set("X-START-TIME", info.StartTime.Format(time.ANSIC))
		w.Header().Set("X-END-TIME", info.EndTime.Format(time.ANSIC))
		w.Header().Set("X-FILENAME", info.Filename)
		w.Header().Set(protocol.SetSizes(info.Sizes))

		rangeHeader := "bytes=" + strconv.FormatUint(info.StartAt, 10) + "-" + strconv.FormatUint(info.EndAt, 10)
		w.Header().Set("RANGE", info.Filename)
		r.Header.Set("RANGE", rangeHeader)

		http.ServeContent(w, r, info.Filename, info.EndTime, info.File)
	}
}
