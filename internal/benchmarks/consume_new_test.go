package benchmarks

import (
	"bytes"
	"fmt"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/haraqa/haraqa"
	"github.com/haraqa/haraqa/pkg/server"
)

func BenchmarkNewConsume(b *testing.B) {
	var err error
	dirNames := make([]string, 1)
	for i := range dirNames {
		dirNames[i], err = os.MkdirTemp("", ".haraqa*")
		if err != nil {
			b.Error(err)
		}
	}
	defer func() {
		for _, dirName := range dirNames {
			if err := os.RemoveAll(dirName); err != nil {
				b.Error(err)
			}
		}
	}()

	haraqaServer, err := server.NewServer(
		server.WithDefaultQueue(dirNames, true, 5000),
	)
	if err != nil {
		b.Fatal(err)
	}
	defer haraqaServer.Close()

	s := httptest.NewServer(haraqaServer)
	s.EnableHTTP2 = true
	defer s.Close()

	c, err := haraqa.NewClient(haraqa.WithURL(s.URL))
	if err != nil {
		b.Fatal(err)
	}

	err = c.CreateTopic("benchtopic")
	if err != nil {
		b.Fatal(err)
	}

	msgs := make([][100]byte, 1000)
	sizes := make([]int64, len(msgs))
	var data []byte
	for i := range msgs {
		copy(msgs[i][:], []byte("something"))
		data = append(data, msgs[i][:]...)
		sizes[i] = int64(len(msgs[i]))
	}

	for i := 0; i < b.N; i += len(msgs) {
		body := bytes.NewBuffer(data)
		err = c.Produce("benchtopic", sizes, body)
		if err != nil {
			b.Fatal(err)
		}
	}

	b.Run("consume 1", benchConsumer(1, c))
	b.Run("consume 10", benchConsumer(10, c))
	b.Run("consume 100", benchConsumer(100, c))
	b.Run("consume 1000", benchConsumer(1000, c))
	fmt.Println("")
	b.Run("go consume 1", benchConsumerN(10, 1, c))
	b.Run("go consume 10", benchConsumerN(10, 10, c))
	b.Run("go consume 100", benchConsumerN(10, 100, c))
	b.Run("go consume 1000", benchConsumerN(10, 1000, c))
	fmt.Println("")
}
