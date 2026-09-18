package contract

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func isTimestamp(fd protoreflect.FieldDescriptor) bool {
	return fd.Kind() == protoreflect.MessageKind && fd.Message().FullName() == "google.protobuf.Timestamp"
}

func isDuration(fd protoreflect.FieldDescriptor) bool {
	return fd.Kind() == protoreflect.MessageKind && fd.Message().FullName() == "google.protobuf.Duration"
}

func valueOf[R proto.Message](r R, fd protoreflect.FieldDescriptor) protoreflect.Value {
	return r.ProtoReflect().Get(fd)
}

func has[R proto.Message](r R, fd protoreflect.FieldDescriptor) bool {
	return r.ProtoReflect().Has(fd)
}

// setValue returns a copy of the resource with the field set to the value.
func setValue[R proto.Message](r R, fd protoreflect.FieldDescriptor, v protoreflect.Value) R {
	copied, _ := proto.Clone(r).(R)
	copied.ProtoReflect().Set(fd, v)

	return copied
}

// clearValue returns a copy of the resource with the field cleared.
func clearValue[R proto.Message](r R, fd protoreflect.FieldDescriptor) R {
	copied, _ := proto.Clone(r).(R)
	copied.ProtoReflect().Clear(fd)

	return copied
}

func timeOf[R proto.Message](r R, fd protoreflect.FieldDescriptor) time.Time {
	stamp, _ := r.ProtoReflect().Get(fd).Message().Interface().(*timestamppb.Timestamp)

	return stamp.AsTime()
}

// differing returns a value the field takes that differs from the current
// one, or false for a kind the specs build no value for.
func differing(fd protoreflect.FieldDescriptor, current protoreflect.Value) (protoreflect.Value, bool) {
	if fd.Kind() == protoreflect.EnumKind {
		// The walk runs from the end, so a state enum yields its final
		// value, which a resource seeded from a valid fixture never reaches.
		values := fd.Enum().Values()
		for i := values.Len() - 1; i >= 0; i-- {
			if number := values.Get(i).Number(); number != 0 && number != current.Enum() {
				return protoreflect.ValueOfEnum(number), true
			}
		}

		return protoreflect.Value{}, false
	}

	candidate, ok := sample(fd)
	if !ok {
		return protoreflect.Value{}, false
	}

	// An unset message field carries no value to compare against.
	if !current.IsValid() || !equalValues(fd, candidate, current) {
		return candidate, true
	}

	if isTimestamp(fd) {
		return protoreflect.ValueOfMessage(timestamppb.New(time.Unix(8, 0)).ProtoReflect()), true
	}

	if isDuration(fd) {
		return protoreflect.ValueOfMessage(durationpb.New(8 * time.Second).ProtoReflect()), true
	}

	return fd.Default(), true
}

// equalValues reports whether both values are equal, comparing messages by
// content.
func equalValues(fd protoreflect.FieldDescriptor, a, b protoreflect.Value) bool {
	if fd.Kind() == protoreflect.MessageKind {
		return proto.Equal(a.Message().Interface(), b.Message().Interface())
	}

	return a.Equal(b)
}

// sample returns a value the field takes that differs from its zero value,
// or false for a kind the specs build no value for.
func sample(fd protoreflect.FieldDescriptor) (protoreflect.Value, bool) {
	switch fd.Kind() {
	case protoreflect.StringKind:
		return protoreflect.ValueOfString("garbage"), true
	case protoreflect.BytesKind:
		return protoreflect.ValueOfBytes([]byte("garbage")), true
	case protoreflect.BoolKind:
		return protoreflect.ValueOfBool(true), true
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return protoreflect.ValueOfInt32(7), true
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return protoreflect.ValueOfInt64(7), true
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return protoreflect.ValueOfUint32(7), true
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return protoreflect.ValueOfUint64(7), true
	case protoreflect.FloatKind:
		return protoreflect.ValueOfFloat32(7.5), true
	case protoreflect.DoubleKind:
		return protoreflect.ValueOfFloat64(7.5), true
	case protoreflect.MessageKind:
		if isTimestamp(fd) {
			return protoreflect.ValueOfMessage(timestamppb.New(time.Unix(7, 0)).ProtoReflect()), true
		}

		if isDuration(fd) {
			return protoreflect.ValueOfMessage(durationpb.New(7 * time.Second).ProtoReflect()), true
		}

		return protoreflect.Value{}, false
	default:
		return protoreflect.Value{}, false
	}
}

// literal returns the value as an AIP-160 filter literal.
func literal(fd protoreflect.FieldDescriptor, v protoreflect.Value) string {
	switch fd.Kind() {
	case protoreflect.StringKind:
		return quote(v.String())
	case protoreflect.EnumKind:
		return string(fd.Enum().Values().ByNumber(v.Enum()).Name())
	case protoreflect.BoolKind:
		return strconv.FormatBool(v.Bool())
	case protoreflect.DoubleKind:
		return strconv.FormatFloat(v.Float(), 'g', -1, 64)
	case protoreflect.FloatKind:
		return strconv.FormatFloat(v.Float(), 'g', -1, 32)
	case protoreflect.MessageKind:
		if isDuration(fd) {
			span, _ := v.Message().Interface().(*durationpb.Duration)

			return quote(durationLiteral(span))
		}

		stamp, _ := v.Message().Interface().(*timestamppb.Timestamp)

		return quote(stamp.AsTime().UTC().Format(time.RFC3339Nano))
	default:
		return fmt.Sprint(v.Interface())
	}
}

// durationLiteral returns the seconds form of google.protobuf.Duration, such
// as 7200s or -1.5s.
func durationLiteral(span *durationpb.Duration) string {
	seconds, nanos := span.GetSeconds(), int64(span.GetNanos())

	sign := ""
	if seconds < 0 || nanos < 0 {
		sign, seconds, nanos = "-", -seconds, -nanos
	}

	text := sign + strconv.FormatInt(seconds, 10)
	if nanos != 0 {
		text += "." + strings.TrimRight(fmt.Sprintf("%09d", nanos), "0")
	}

	return text + "s"
}

// quote returns the text as a filter string literal.
func quote(text string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(text) + `"`
}

// less reports whether a orders before b, and whether the kind orders at all.
func less(fd protoreflect.FieldDescriptor, a, b protoreflect.Value) (bool, bool) {
	switch fd.Kind() {
	case protoreflect.StringKind:
		return a.String() < b.String(), true
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
		protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return a.Int() < b.Int(), true
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind, protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return a.Uint() < b.Uint(), true
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return a.Float() < b.Float(), true
	case protoreflect.MessageKind:
		if isDuration(fd) {
			first, _ := a.Message().Interface().(*durationpb.Duration)
			second, _ := b.Message().Interface().(*durationpb.Duration)

			return first.AsDuration() < second.AsDuration(), true
		}

		if !isTimestamp(fd) {
			return false, false
		}

		first, _ := a.Message().Interface().(*timestamppb.Timestamp)
		second, _ := b.Message().Interface().(*timestamppb.Timestamp)

		return first.AsTime().Before(second.AsTime()), true
	default:
		return false, false
	}
}

// textual reports whether the field takes a wildcard, which a filter offers
// text alone. A resource reference is compared whole.
func textual(fd protoreflect.FieldDescriptor) bool {
	return fd.Kind() == protoreflect.StringKind &&
		!proto.HasExtension(fd.Options(), annotations.E_ResourceReference)
}
