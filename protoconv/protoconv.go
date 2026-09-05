package protoconv

import (
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TimeToProto returns nil for the zero time.
func TimeToProto(t time.Time) *timestamppb.Timestamp {
	if !t.IsZero() {
		return timestamppb.New(t)
	}

	return nil
}

// TimeFromProto returns nil for a nil timestamp.
func TimeFromProto(ts *timestamppb.Timestamp) *time.Time {
	if ts == nil {
		return nil
	}

	return new(ts.AsTime())
}

// DurationToProto returns nil for a nil duration.
func DurationToProto(d *time.Duration) *durationpb.Duration {
	if d == nil {
		return nil
	}

	return durationpb.New(*d)
}

// DurationFromProto returns nil for a nil duration, and loses nothing the wire
// carried.
func DurationFromProto(d *durationpb.Duration) *time.Duration {
	if d == nil {
		return nil
	}

	return new(d.AsDuration())
}
