package contract

import (
	"strings"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/descriptorpb"
)

// shape is what the message descriptor says about the resource.
type shape struct {
	// Singular and Plural are the resource's singular, such as "book",
	// and the collection segment of its name, such as "books".
	Singular string
	Plural   string

	Fields     map[string]protoreflect.FieldDescriptor
	Creatable  []protoreflect.FieldDescriptor
	Writable   []protoreflect.FieldDescriptor
	Required   []protoreflect.FieldDescriptor
	Optional   []protoreflect.FieldDescriptor
	Immutable  []protoreflect.FieldDescriptor
	OutputOnly []protoreflect.FieldDescriptor

	NameField       protoreflect.FieldDescriptor
	EtagField       protoreflect.FieldDescriptor
	CreateTimeField protoreflect.FieldDescriptor
	UpdateTimeField protoreflect.FieldDescriptor
	DeleteTimeField protoreflect.FieldDescriptor

	message proto.Message
}

func newShape[R proto.Message]() shape {
	var zero R

	desc := zero.ProtoReflect().Descriptor()

	s := shape{
		Fields:  map[string]protoreflect.FieldDescriptor{},
		message: zero.ProtoReflect().New().Interface(),
	}

	opts, _ := desc.Options().(*descriptorpb.MessageOptions)
	if res, ok := proto.GetExtension(opts, annotations.E_Resource).(*annotations.ResourceDescriptor); ok && res != nil {
		s.Singular = res.GetSingular()

		if patterns := res.GetPattern(); len(patterns) > 0 {
			segments := strings.Split(patterns[0], "/")
			s.Plural = segments[len(segments)-2]
		}
	}

	if s.Singular == "" {
		s.Singular = strings.ToLower(string(desc.Name()))
	}

	fields := desc.Fields()
	for i := range fields.Len() {
		fd := fields.Get(i)
		s.Fields[string(fd.Name())] = fd

		switch fd.Name() {
		case "name":
			s.NameField = fd
		case "etag":
			s.EtagField = fd
		case "create_time":
			s.CreateTimeField = fd
		case "update_time":
			s.UpdateTimeField = fd
		case "delete_time":
			s.DeleteTimeField = fd
		}

		behaviors := behaviorsOf(fd)

		switch {
		case fd.Name() == "name" || fd.Name() == "etag":
		case behaviors[annotations.FieldBehavior_OUTPUT_ONLY]:
			s.OutputOnly = append(s.OutputOnly, fd)
		case behaviors[annotations.FieldBehavior_IDENTIFIER]:
		case !settable(fd):
		case behaviors[annotations.FieldBehavior_IMMUTABLE]:
			s.Immutable = append(s.Immutable, fd)
			s.Creatable = append(s.Creatable, fd)

			if behaviors[annotations.FieldBehavior_REQUIRED] {
				s.Required = append(s.Required, fd)
			}
		default:
			s.Writable = append(s.Writable, fd)
			s.Creatable = append(s.Creatable, fd)

			if behaviors[annotations.FieldBehavior_REQUIRED] {
				s.Required = append(s.Required, fd)
			} else {
				s.Optional = append(s.Optional, fd)
			}
		}
	}

	return s
}

func behaviorsOf(fd protoreflect.FieldDescriptor) map[annotations.FieldBehavior]bool {
	found := map[annotations.FieldBehavior]bool{}

	opts, _ := fd.Options().(*descriptorpb.FieldOptions)
	if declared, ok := proto.GetExtension(opts, annotations.E_FieldBehavior).([]annotations.FieldBehavior); ok {
		for _, behavior := range declared {
			found[behavior] = true
		}
	}

	return found
}

// Comparing returns the cmp options that compare the given fields of the
// resource and ignore every other one.
func (s shape) Comparing(fields ...protoreflect.FieldDescriptor) []cmp.Option {
	compared := map[protoreflect.Name]bool{}
	for _, fd := range fields {
		compared[fd.Name()] = true
	}

	ignored := make([]protoreflect.Name, 0, len(s.Fields))
	for _, fd := range s.Fields {
		if !compared[fd.Name()] {
			ignored = append(ignored, fd.Name())
		}
	}

	return []cmp.Option{protocmp.Transform(), protocmp.IgnoreFields(s.message, ignored...)}
}

// without returns the fields except the given one.
func without(fields []protoreflect.FieldDescriptor, except protoreflect.FieldDescriptor) []protoreflect.FieldDescriptor {
	kept := make([]protoreflect.FieldDescriptor, 0, len(fields))
	for _, fd := range fields {
		if fd != except {
			kept = append(kept, fd)
		}
	}

	return kept
}

// settable reports whether the specs can build a value for the field.
func settable(fd protoreflect.FieldDescriptor) bool {
	if fd.IsList() || fd.IsMap() {
		return false
	}

	return fd.Kind() != protoreflect.MessageKind || isTimestamp(fd)
}

// enforceable reports whether a missing value of the field is observable,
// which a proto3 scalar without presence is not.
func enforceable(fd protoreflect.FieldDescriptor) bool {
	switch fd.Kind() {
	case protoreflect.StringKind, protoreflect.BytesKind, protoreflect.EnumKind, protoreflect.MessageKind:
		return true
	default:
		return fd.HasPresence()
	}
}
