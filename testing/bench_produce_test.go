package testing

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/haraqa/haraqa"
	"github.com/haraqa/haraqa/broker"
)

func BenchmarkProduce(b *testing.B) {
	defer os.RemoveAll(".haraqa")
	brkr, err := broker.NewBroker(broker.DefaultConfig)
	if err != nil {
		b.Fatal(err)
	}
	go func() {
		err := brkr.Listen()
		if err != nil {
			b.Fatal(err)
		}
	}()
	defer brkr.Close()
	fmt.Println("")
	b.Run("produce 1", benchProducer(1))
	b.Run("produce 10", benchProducer(10))
	b.Run("produce 100", benchProducer(100))
	b.Run("produce 1000", benchProducer(1000))
	fmt.Println("")
	b.Run("produce loop 1", benchProducerLoop(1))
	b.Run("produce loop 10", benchProducerLoop(10))
	b.Run("produce loop 100", benchProducerLoop(100))
	b.Run("produce loop 1000", benchProducerLoop(1000))
}

func benchProducer(batchSize int) func(b *testing.B) {
	return func(b *testing.B) {
		client, err := haraqa.NewClient(haraqa.DefaultConfig)
		if err != nil {
			b.Fatal(err)
		}
		ctx := context.Background()
		topic := []byte("something")
		client.CreateTopic(ctx, topic)

		msgs := make([][]byte, batchSize)
		for i := range msgs {
			msgs[i] = make([]byte, 100)
			rand.Read(msgs[i])
		}

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i += batchSize {
			err := client.Produce(ctx, topic, msgs...)
			if err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
	}
}

func benchProducerLoop(batchSize int) func(b *testing.B) {
	return func(b *testing.B) {
		client, err := haraqa.NewClient(haraqa.DefaultConfig)
		if err != nil {
			b.Fatal(err)
		}
		ctx := context.Background()
		topic := []byte("something")
		client.CreateTopic(ctx, topic)

		msg := make([]byte, 100)
		rand.Read(msg)

		ch := make(chan haraqa.ProduceMsg, batchSize)
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			err := client.ProduceLoop(ctx, topic, ch)
			if err != nil {
				b.Fatal(err)
			}
			wg.Done()
		}()

		errs := make([]chan error, b.N)
		for i := range errs {
			errs[i] = make(chan error, 1)
		}

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			ch <- haraqa.ProduceMsg{
				Msg: msg,
				Err: errs[i],
			}
		}
		close(ch)
		wg.Wait()

		b.StopTimer()
	}
}