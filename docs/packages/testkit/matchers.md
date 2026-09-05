# testkit/matchers

The `testkit/matchers` package holds gomega matchers for the AIP-193 details an
error carries, so a spec asserts on one in a line.

## Usage

A spec dot-imports the package alongside gomega.

```go
Expect(err).To(HaveViolatedField("name"))
Expect(err).To(HaveReason("BOOK_MISSING"))
```

`HaveViolatedField` and `HaveViolationDescription` read the `BadRequest`.
`HaveReason`, `HaveErrorDomain` and `HaveErrorMetadata` read the `ErrorInfo`.

## Mechanics

Each matcher converts the actual value to a gRPC status and finds the first
detail of the type it needs. A failure message names what the status carried
instead, so a spec that fails says which field was violated rather than only
that the expected one was not.
