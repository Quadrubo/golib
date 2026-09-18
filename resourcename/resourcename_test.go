package resourcename_test

import (
	"errors"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/quadrubo/golib/resourcename"
)

func TestResourcename(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Resourcename Suite")
}

var (
	room = resourcename.MustCompile("rooms/*")
	task = resourcename.MustCompile("lists/*/tasks/*")

	serverConfig = resourcename.Singleton("serverConfig")
	settings     = resourcename.Singleton("users/*/settings")
)

var _ = Describe("MustCompile", func() {
	It("panics on a pattern ending in a collection", func() {
		Expect(func() { resourcename.MustCompile("rooms") }).To(Panic())
	})

	It("panics on a pattern naming an id where a collection belongs", func() {
		Expect(func() { resourcename.MustCompile("rooms/*/*/*") }).To(Panic())
	})

	It("panics on a pattern spelling out an id segment", func() {
		Expect(func() { resourcename.MustCompile("rooms/room") }).To(Panic())
	})

	It("panics on an empty pattern", func() {
		Expect(func() { resourcename.MustCompile("") }).To(Panic())
	})
})

var _ = Describe("Parse", func() {
	It("returns the id of a top-level name", func() {
		Expect(room.Parse("rooms/studio")).To(Equal([]string{"studio"}))
	})

	It("returns every id of a nested name", func() {
		Expect(task.Parse("lists/inbox/tasks/milk")).To(Equal([]string{"inbox", "milk"}))
	})

	It("rejects a name in another collection", func() {
		_, err := room.Parse("users/studio")

		Expect(err).To(HaveOccurred())
	})

	It("rejects a name carrying more segments than the pattern", func() {
		_, err := room.Parse("rooms/studio/tasks/milk")

		Expect(err).To(HaveOccurred())
	})

	It("rejects a name carrying fewer segments than the pattern", func() {
		_, err := task.Parse("lists/inbox")

		Expect(err).To(HaveOccurred())
	})

	It("returns the id of a name a uuid identifies", func() {
		Expect(room.Parse("rooms/0199c0de-7a1b-7c3d-8e4f-a5b6c7d8e9f0")).
			To(Equal([]string{"0199c0de-7a1b-7c3d-8e4f-a5b6c7d8e9f0"}))
	})

	It("rejects an id carrying a character the segment format leaves out", func() {
		_, err := room.Parse("rooms/STUDIO")

		Expect(err).To(HaveOccurred())
	})

	It("rejects an id longer than the segment format allows", func() {
		_, err := room.Parse("rooms/" + strings.Repeat("a", 64))

		Expect(err).To(HaveOccurred())
	})

	It("rejects an empty id segment", func() {
		_, err := room.Parse("rooms/")

		Expect(err).To(HaveOccurred())
	})

	It("rejects a nested name whose parent id is empty", func() {
		_, err := task.Parse("lists//tasks/milk")

		Expect(err).To(HaveOccurred())
	})

	It("reports the name it was given and the pattern it wanted", func() {
		_, err := room.Parse("users/studio")

		var nameErr *resourcename.Error
		Expect(errors.As(err, &nameErr)).To(BeTrue())
		Expect(nameErr.Name).To(Equal("users/studio"))
		Expect(nameErr.Pattern).To(Equal("rooms/*"))
	})

	It("names the pattern when it was given nothing", func() {
		_, err := room.Parse("")

		Expect(err).To(MatchError("resource name is required, expected rooms/*"))
	})
})

var _ = Describe("Format", func() {
	It("composes a top-level name", func() {
		Expect(room.Format("studio")).To(Equal("rooms/studio"))
	})

	It("composes a nested name", func() {
		Expect(task.Format("inbox", "milk")).To(Equal("lists/inbox/tasks/milk"))
	})

	It("returns what Parse reads back", func() {
		Expect(task.Parse(task.Format("inbox", "milk"))).To(Equal([]string{"inbox", "milk"}))
	})

	It("panics on fewer ids than the pattern takes", func() {
		Expect(func() { task.Format("inbox") }).
			To(PanicWith(`resourcename: pattern lists/*/tasks/* cannot format ["inbox"]`))
	})

	It("panics on more ids than the pattern takes", func() {
		Expect(func() { room.Format("studio", "milk") }).
			To(PanicWith(`resourcename: pattern rooms/* cannot format ["studio" "milk"]`))
	})
})

var _ = Describe("Singleton", func() {
	It("panics on a pattern closing on a wildcard", func() {
		Expect(func() { resourcename.Singleton("rooms/*") }).To(Panic())
	})

	It("panics on a pattern closing on nothing", func() {
		Expect(func() { resourcename.Singleton("users/*/") }).To(Panic())
	})

	It("panics on a pattern spelling out an id segment", func() {
		Expect(func() { resourcename.Singleton("users/alice/settings") }).To(Panic())
	})

	It("panics on an empty pattern", func() {
		Expect(func() { resourcename.Singleton("") }).To(Panic())
	})
})

var _ = Describe("Parse of a singleton", func() {
	It("returns no id for a top-level singleton", func() {
		Expect(serverConfig.Parse("serverConfig")).To(Equal([]string{}))
	})

	It("returns the id of the parent a nested singleton sits under", func() {
		Expect(settings.Parse("users/alice/settings")).To(Equal([]string{"alice"}))
	})

	It("rejects a name differing from the singleton in case", func() {
		_, err := serverConfig.Parse("serverconfig")

		Expect(err).To(HaveOccurred())
	})

	It("rejects a name carrying more segments than the pattern", func() {
		_, err := serverConfig.Parse("serverConfig/rooms/studio")

		Expect(err).To(HaveOccurred())
	})

	It("rejects a nested name closing on another singleton", func() {
		_, err := settings.Parse("users/alice/profile")

		Expect(err).To(HaveOccurred())
	})

	It("rejects a nested name whose parent id leaves the segment format", func() {
		_, err := settings.Parse("users/ALICE/settings")

		Expect(err).To(HaveOccurred())
	})

	It("names the pattern when it was given nothing", func() {
		_, err := serverConfig.Parse("")

		Expect(err).To(MatchError("resource name is required, expected serverConfig"))
	})
})

var _ = Describe("Format of a singleton", func() {
	It("composes a top-level singleton from no id", func() {
		Expect(serverConfig.Format()).To(Equal("serverConfig"))
	})

	It("composes a nested singleton from the id of its parent", func() {
		Expect(settings.Format("alice")).To(Equal("users/alice/settings"))
	})

	It("returns what Parse reads back", func() {
		Expect(settings.Parse(settings.Format("alice"))).To(Equal([]string{"alice"}))
	})

	It("panics on more ids than the pattern takes", func() {
		Expect(func() { serverConfig.Format("alice") }).
			To(PanicWith(`resourcename: pattern serverConfig cannot format ["alice"]`))
	})
})

var _ = Describe("FillUUIDv7", func() {
	It("sets a UUID v7 on an empty id", func() {
		var id string

		Expect(resourcename.FillUUIDv7(&id)).To(Succeed())
		Expect(id).To(HaveLen(36))
		Expect(id[14]).To(Equal(byte('7')))
	})

	It("keeps a given id", func() {
		id := "alice"

		Expect(resourcename.FillUUIDv7(&id)).To(Succeed())
		Expect(id).To(Equal("alice"))
	})

	It("sets a different id each time", func() {
		var first, second string

		Expect(resourcename.FillUUIDv7(&first)).To(Succeed())
		Expect(resourcename.FillUUIDv7(&second)).To(Succeed())
		Expect(first).NotTo(Equal(second))
	})
})
