package broker

import (
	"context"
	"log"
	"os"

	"github.com/haraqa/haraqa/protocol"
	pb "github.com/haraqa/haraqa/protocol"
)

// CreateTopic implements protocol.HaraqaServer CreateTopic
func (b *Broker) CreateTopic(ctx context.Context, in *pb.CreateTopicRequest) (*pb.CreateTopicResponse, error) {
	err := b.config.Queue.CreateTopic(in.GetTopic())
	if err != nil {
		return &pb.CreateTopicResponse{Meta: &pb.Meta{OK: false, ErrorMsg: err.Error()}}, nil
	}

	return &pb.CreateTopicResponse{Meta: &pb.Meta{OK: true}}, nil
}

// DeleteTopic implements protocol.HaraqaServer CreateTopic
func (b *Broker) DeleteTopic(ctx context.Context, in *pb.DeleteTopicRequest) (*pb.DeleteTopicResponse, error) {
	err := b.config.Queue.DeleteTopic(in.GetTopic())
	if err != nil {
		return &pb.DeleteTopicResponse{Meta: &pb.Meta{OK: false, ErrorMsg: err.Error()}}, nil
	}

	return &pb.DeleteTopicResponse{Meta: &pb.Meta{OK: true}}, nil
}

// ListTopics implements protocol.HaraqaServer ListTopics
func (b *Broker) ListTopics(ctx context.Context, in *pb.ListTopicsRequest) (*pb.ListTopicsResponse, error) {
	topics, err := b.config.Queue.ListTopics()
	if err != nil {
		return &pb.ListTopicsResponse{Meta: &pb.Meta{OK: false, ErrorMsg: err.Error()}}, nil
	}

	return &pb.ListTopicsResponse{Meta: &pb.Meta{OK: true}, Topics: topics}, nil
}

// Produce implements protocol.HaraqaServer Produce
func (b *Broker) Produce(ctx context.Context, in *pb.ProduceRequest) (*pb.ProduceResponse, error) {
	ok := b.sendToDataTriggerChannel(in.GetUuid(), dataTrigger{
		incoming: true,
		topic:    in.GetTopic(),
		sizes:    in.GetMsgSizes(),
	})
	if !ok {
		return &pb.ProduceResponse{Meta: &pb.Meta{OK: false, ErrorMsg: "data connection not found"}}, nil
	}

	return &pb.ProduceResponse{Meta: &pb.Meta{OK: true}}, nil
}

// Consume implements protocol.HaraqaServer Consume
func (b *Broker) Consume(ctx context.Context, in *pb.ConsumeRequest) (*pb.ConsumeResponse, error) {
	filename, startAt, msgSizes, err := b.config.Queue.ConsumeInfo(in.GetTopic(), in.GetOffset(), in.GetMaxBatchSize())
	if err != nil {
		return &pb.ConsumeResponse{Meta: &pb.Meta{OK: false, ErrorMsg: err.Error()}}, nil
	}

	if len(msgSizes) != 0 {
		ok := b.sendToDataTriggerChannel(in.GetUuid(), dataTrigger{
			incoming:  false,
			topic:     in.GetTopic(),
			filename:  filename,
			startAt:   startAt,
			totalSize: sum(msgSizes),
		})
		if !ok {
			return &pb.ConsumeResponse{Meta: &pb.Meta{OK: false, ErrorMsg: "data connection not found"}}, nil
		}
	}

	return &pb.ConsumeResponse{
		Meta:     &pb.Meta{OK: true},
		MsgSizes: msgSizes,
	}, nil
}

func sum(s []int64) int64 {
	var out int64
	for _, v := range s {
		out += v
	}
	return out
}

// TruncateTopic implements protocol.HaraqaServer TruncateTopic
func (b *Broker) TruncateTopic(ctx context.Context, in *pb.TruncateTopicRequest) (*pb.TruncateTopicResponse, error) {
	log.Printf("Received: %v", in.GetTopic())
	return &pb.TruncateTopicResponse{Meta: &pb.Meta{OK: true}}, nil
}

// CloseConnection implements protocol.HaraqaServer CloseConnection
func (b *Broker) CloseConnection(ctx context.Context, in *pb.CloseRequest) (*pb.CloseResponse, error) {
	b.closeDataTriggerChannel(in.GetUuid())

	return &pb.CloseResponse{Meta: &pb.Meta{OK: true}}, nil
}

// Offsets implements protocol.HaraqaServer Offset
func (b *Broker) Offsets(ctx context.Context, in *pb.OffsetRequest) (*pb.OffsetResponse, error) {
	min, max, err := b.config.Queue.Offsets(in.GetTopic())
	if err != nil {
		if err == os.ErrNotExist {
			err = protocol.ErrTopicDoesNotExist
		}
		return &pb.OffsetResponse{Meta: &pb.Meta{OK: false, ErrorMsg: err.Error()}}, nil
	}

	return &pb.OffsetResponse{Meta: &pb.Meta{OK: true}, MinOffset: min, MaxOffset: max}, nil
}
