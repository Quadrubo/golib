package queryfield_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/quadrubo/golib/queryfield"
)

func TestQueryfield(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Queryfield Suite")
}

type row struct {
	Name     string
	Created  time.Time
	Capacity int64
	Hidden   bool
	Weight   float64
	Phase    string
	Colour   *string
	Closed   *time.Time
	Featured *bool
	Rating   *float64
	Offset   time.Duration
	Pause    *time.Duration
}

var created = time.Date(2026, time.August, 3, 12, 0, 0, 123456000, time.UTC)

var sample = row{
	Name: "Alto", Created: created, Capacity: 12, Hidden: true, Weight: 0.1, Phase: "open",
	Offset: 2 * time.Hour,
}

// roundTrip encodes the row and parses the value back, failing the spec where the
// column holds NULL.
func roundTrip[T any](field queryfield.Field[T], of T) any {
	GinkgoHelper()

	encoded := field.Cursor(of)
	Expect(encoded).ToNot(BeNil())

	parsed, err := field.ParseCursor(*encoded)
	Expect(err).ToNot(HaveOccurred())

	return parsed
}

var _ = Describe("Text", func() {
	field := queryfield.Text("display_name", func(r row) string { return r.Name })

	It("declares the column and the kind", func() {
		Expect(field.Column).To(Equal("display_name"))
		Expect(field.Kind).To(Equal(queryfield.KindText))
	})

	It("declares a column that never holds NULL", func() {
		Expect(field.Nullable()).To(BeFalse())
	})

	It("tells a field it returned from one a caller wrote as a literal", func() {
		Expect(field.Declared()).To(BeTrue())
		Expect(queryfield.Field[row]{}.Declared()).To(BeFalse())
		Expect(queryfield.Field[row]{Column: "display_name"}.Declared()).To(BeFalse())
	})

	It("parses back the value it encoded", func() {
		Expect(roundTrip(field, sample)).To(Equal("Alto"))
	})
})

var _ = Describe("Time", func() {
	field := queryfield.Time("create_time", func(r row) time.Time { return r.Created })

	It("declares the column and the kind", func() {
		Expect(field.Column).To(Equal("create_time"))
		Expect(field.Kind).To(Equal(queryfield.KindTime))
	})

	It("parses back the instant it encoded", func() {
		Expect(roundTrip(field, sample)).To(BeTemporally("==", created))
	})

	It("drops the nanoseconds below microsecond resolution", func() {
		nanos := row{Created: created.Add(999 * time.Nanosecond)}

		Expect(field.Cursor(nanos)).To(Equal(field.Cursor(sample)))
	})

	It("encodes the instant rather than the zone it is read in", func() {
		zoned := row{Created: created.In(time.FixedZone("test", 3600))}

		Expect(field.Cursor(zoned)).To(Equal(field.Cursor(sample)))
	})

	It("returns an error for a cursor value that is not epoch microseconds", func() {
		Expect(field.ParseCursor("now")).Error().To(HaveOccurred())
	})
})

var _ = Describe("Int", func() {
	field := queryfield.Int("capacity", func(r row) int64 { return r.Capacity })

	It("declares the column and the kind", func() {
		Expect(field.Column).To(Equal("capacity"))
		Expect(field.Kind).To(Equal(queryfield.KindInt))
	})

	It("parses back the number it encoded", func() {
		Expect(roundTrip(field, sample)).To(Equal(int64(12)))
	})

	It("returns an error for a cursor value that is not a number", func() {
		Expect(field.ParseCursor("twelve")).Error().To(HaveOccurred())
	})
})

var _ = Describe("Float", func() {
	field := queryfield.Float("weight", func(r row) float64 { return r.Weight })

	It("declares the column and the kind", func() {
		Expect(field.Column).To(Equal("weight"))
		Expect(field.Kind).To(Equal(queryfield.KindFloat))
	})

	It("parses back the number it encoded", func() {
		Expect(roundTrip(field, sample)).To(Equal(0.1))
	})

	It("encodes the shortest decimal that parses back to the same bits", func() {
		third := row{Weight: 1.0 / 3.0}

		Expect(*field.Cursor(third)).To(Equal("0.3333333333333333"))
		Expect(roundTrip(field, third)).To(Equal(1.0 / 3.0))
	})

	It("returns an error for a cursor value that is not a number", func() {
		Expect(field.ParseCursor("twelve")).Error().To(HaveOccurred())
	})
})

var _ = Describe("Bool", func() {
	field := queryfield.Bool("hidden", func(r row) bool { return r.Hidden })

	It("declares the column and the kind", func() {
		Expect(field.Column).To(Equal("hidden"))
		Expect(field.Kind).To(Equal(queryfield.KindBool))
	})

	It("parses back the word it encoded", func() {
		Expect(roundTrip(field, sample)).To(Equal(true))
		Expect(roundTrip(field, row{})).To(Equal(false))
	})

	It("returns an error for the spellings strconv widens true to", func() {
		for _, raw := range []string{"1", "t", "T", "TRUE", "True", "yes"} {
			Expect(field.ParseCursor(raw)).Error().To(HaveOccurred())
			Expect(field.ParseLiteral(raw)).Error().To(HaveOccurred())
		}
	})
})

var _ = Describe("Enum", func() {
	field := queryfield.Enum("phase", map[string]string{
		"PHASE_OPEN":   "open",
		"PHASE_CLOSED": "closed",
	}, func(r row) string { return r.Phase })

	It("declares the column and the kind", func() {
		Expect(field.Column).To(Equal("phase"))
		Expect(field.Kind).To(Equal(queryfield.KindEnum))
	})

	It("parses a literal to the word the column holds", func() {
		Expect(field.ParseLiteral("PHASE_OPEN")).To(Equal("open"))
	})

	It("returns an error for a spelling outside the enum, the stored word too", func() {
		Expect(field.ParseLiteral("PHASE_LOCKED")).Error().To(HaveOccurred())
		Expect(field.ParseLiteral("open")).Error().To(HaveOccurred())
	})

	It("parses back the cursor value it encoded", func() {
		Expect(roundTrip(field, sample)).To(Equal("open"))
	})
})

var _ = Describe("Duration", func() {
	field := queryfield.Duration("offset_seconds", time.Second, func(r row) time.Duration { return r.Offset })

	It("declares the column and the kind", func() {
		Expect(field.Column).To(Equal("offset_seconds"))
		Expect(field.Kind).To(Equal(queryfield.KindDuration))
	})

	It("encodes the cursor as the count of units and parses it back", func() {
		Expect(*field.Cursor(sample)).To(Equal("7200"))
		Expect(roundTrip(field, sample)).To(Equal(int64(7200)))
	})

	DescribeTable("parses a literal in the seconds form to the count of units",
		func(raw string, want int64) {
			Expect(field.ParseLiteral(raw)).To(Equal(want))
		},
		Entry("whole seconds", "7200s", int64(7200)),
		Entry("negative seconds", "-7200s", int64(-7200)),
		Entry("zero", "0s", int64(0)),
	)

	It("parses a fraction the unit holds", func() {
		millis := queryfield.Duration("pause_millis", time.Millisecond, func(r row) time.Duration { return r.Offset })

		Expect(millis.ParseLiteral("1.5s")).To(Equal(int64(1500)))
	})

	It("returns an error for a fraction finer than the unit", func() {
		Expect(field.ParseLiteral("1.5s")).Error().To(MatchError("it is not a multiple of 1s"))
	})

	DescribeTable("returns an error for a literal outside the seconds form",
		func(raw string) {
			Expect(field.ParseLiteral(raw)).Error().To(
				MatchError(`it is not a duration in seconds, such as "7200s"`))
		},
		Entry("a bare number", "7200"),
		Entry("the Go spelling", "2h0m0s"),
		Entry("a fraction without the suffix", "1.5"),
		Entry("an exponent", "1e3s"),
		Entry("the suffix alone", "s"),
		Entry("ten fraction digits", "1.0000000005s"),
	)

	It("returns an error for a count of seconds a duration does not hold", func() {
		Expect(field.ParseLiteral("9223372037s")).Error().To(MatchError("it is out of range"))
	})

	It("panics on a unit that is not positive", func() {
		Expect(func() {
			queryfield.Duration("offset_seconds", 0, func(r row) time.Duration { return r.Offset })
		}).To(PanicWith("queryfield: the duration unit is not positive"))
	})
})

var _ = Describe("A nullable field", func() {
	colour := queryfield.Text("colour", func(r row) *string { return r.Colour })
	closed := queryfield.Time("closed_time", func(r row) *time.Time { return r.Closed })
	featured := queryfield.Bool("featured", func(r row) *bool { return r.Featured })
	rating := queryfield.Float("rating", func(r row) *float64 { return r.Rating })
	pause := queryfield.Duration("pause_seconds", time.Second, func(r row) *time.Duration { return r.Pause })

	It("declares a column that holds NULL", func() {
		Expect(colour.Nullable()).To(BeTrue())
		Expect(featured.Nullable()).To(BeTrue())
		Expect(rating.Nullable()).To(BeTrue())
		Expect(pause.Nullable()).To(BeTrue())
	})

	It("declares the literal COALESCE puts in place of NULL", func() {
		Expect(colour.Zero()).To(Equal("''"))
		Expect(closed.Zero()).To(Equal("'-infinity'::timestamptz"))
		Expect(featured.Zero()).To(Equal("false"))
		Expect(rating.Zero()).To(Equal("'-Infinity'::float8"))
		Expect(pause.Zero()).To(Equal("0"))
	})

	It("encodes no cursor value for a row holding NULL", func() {
		Expect(colour.Cursor(row{})).To(BeNil())
		Expect(closed.Cursor(row{})).To(BeNil())
		Expect(featured.Cursor(row{})).To(BeNil())
		Expect(rating.Cursor(row{})).To(BeNil())
		Expect(pause.Cursor(row{})).To(BeNil())
	})

	It("parses back the value a row does hold", func() {
		held := "green"
		flag := true
		stars := 4.5
		span := 90 * time.Second

		Expect(roundTrip(colour, row{Colour: &held})).To(Equal("green"))
		Expect(roundTrip(closed, row{Closed: &created})).To(BeTemporally("==", created))
		Expect(roundTrip(featured, row{Featured: &flag})).To(Equal(true))
		Expect(roundTrip(rating, row{Rating: &stars})).To(Equal(4.5))
		Expect(roundTrip(pause, row{Pause: &span})).To(Equal(int64(90)))
	})
})
