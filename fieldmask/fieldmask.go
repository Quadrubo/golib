package fieldmask

import (
	"fmt"
	"slices"

	"buf.build/go/protovalidate"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

type Mask map[string]struct{}

func (m Mask) Has(path string) bool {
	_, ok := m[path]

	return ok
}

// Paths names the fields an update takes. An immutable path may be selected
// with an unchanged value, which AIP-203 permits, so the service compares the
// value rather than refusing the path.
type Paths struct {
	Writable  []string
	Immutable []string
}

// Parse returns the paths the mask selects, or every writable path when the
// mask is omitted, which AIP-134 allows.
func Parse(mask *fieldmaskpb.FieldMask, paths Paths) (Mask, error) {
	selected := mask.GetPaths()
	if len(selected) == 0 {
		selected = paths.Writable
	}

	parsed := make(Mask, len(selected))
	for _, path := range selected {
		if !slices.Contains(paths.Writable, path) && !slices.Contains(paths.Immutable, path) {
			return nil, fmt.Errorf("fieldmask: unsupported field %q", path)
		}

		parsed[path] = struct{}{}
	}

	return parsed, nil
}

// Filter limits protovalidate to the fields the mask selects, so the rules a
// proto declares run against a partial message.
func Filter(mask Mask) protovalidate.Filter {
	return protovalidate.FilterFunc(func(_ protoreflect.Message, descriptor protoreflect.Descriptor) bool {
		field, ok := descriptor.(protoreflect.FieldDescriptor)
		if !ok {
			return true
		}

		return mask.Has(string(field.Name()))
	})
}
