# protoconv

The `protoconv` package converts the values a model holds to the shapes a proto
carries, and back.

## Usage

`TimeToProto` and `TimeFromProto` convert between `time.Time` and
`*timestamppb.Timestamp`. `DurationToProto` and `DurationFromProto` do the same
for `time.Duration` and `*durationpb.Duration`.

```go
book.CreateTime = protoconv.TimeToProto(row.CreateTime)
```

Every conversion returns a pointer, and nil stands for absent. A zero
`time.Time` converts to a nil `Timestamp` rather than to the epoch, so an unset
time is absent from the response instead of reading as 1970.

Nothing here narrows a value. A column holding whole seconds is narrowed by the
caller that knows its width.
