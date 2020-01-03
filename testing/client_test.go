package testing

import (
	"context"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/haraqa/haraqa"
	"github.com/haraqa/haraqa/broker"
)

func TestClient(t *testing.T) {

	var wg sync.WaitGroup
	wg.Add(2)
	mockQueue := broker.NewMockQueue(gomock.NewController(t))
	mockQueue.EXPECT().Produce(gomock.Any(), []byte("world"), []int64{5}).
		DoAndReturn(func(tcpConn *os.File, topic []byte, msgSizes []int64) error {
			var b [5]byte
			_, err := io.ReadFull(tcpConn, b[:])
			if err != nil {
				t.Fatal(err)
			}
			if string(b[:]) != "hello" {
				t.Fatal(string(b[:]))
			}
			wg.Done()
			return nil
		})
	mockQueue.EXPECT().ConsumeData([]byte("world"), int64(20), int64(10)).
		Return([]byte("filename"), int64(100), []int64{5, 6, 9}, nil)

	mockQueue.EXPECT().Consume(gomock.Any(), []byte("world"), []byte("filename"), int64(100), int64(20)).
		DoAndReturn(func(tcpConn *os.File, topic, filename []byte, startAt, totalSize int64) error {

			_, err := tcpConn.Write([]byte("hello there consumer"))
			if err != nil {
				t.Fatal(err)
			}
			wg.Done()
			return nil
		})

	cfg := broker.DefaultConfig
	cfg.Queue = mockQueue
	b, err := broker.NewBroker(cfg)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		err := b.Listen()
		if err != nil {
			t.Fatal(err)
		}
	}()
	defer b.Close()

	t.Run("producer", testProduce)
	t.Run("consumer", testConsumer)
	wg.Wait()
}

func testProduce(t *testing.T) {
	config := haraqa.DefaultConfig
	config.UnixSocket = "/tmp/haraqa.sock"

	client, err := haraqa.NewClient(config)
	for err != nil {
		client, err = haraqa.NewClient(config)
	}
	ctx := context.Background()
	err = client.Produce(ctx, []byte("world"), []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
}

func testConsumer(t *testing.T) {
	config := haraqa.DefaultConfig
	config.Timeout = time.Second * 1
	client, err := haraqa.NewClient(config)
	for err != nil {
		t.Log(err)
		client, err = haraqa.NewClient(config)
	}
	ctx := context.Background()
	resp := haraqa.ConsumeResponse{}
	err = client.Consume(ctx, []byte("world"), 20, 10, &resp)
	if err != nil {
		t.Fatal(err)
	}

	msg, err := resp.Next()
	if err != nil {
		t.Fatal(err)
	}
	if string(msg) != "hello" {
		t.Fatal(string(msg))
	}

	batch, err := resp.Batch()
	if err != nil {
		t.Fatal(err)
	}
	if string(batch[0]) != " there" {
		t.Fatal(string(batch[0]))
	}
	if string(batch[1]) != " consumer" {
		t.Fatal(string(batch[1]))
	}
}