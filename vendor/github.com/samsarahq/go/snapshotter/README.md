# Snapshotter

Snapshotter makes it easy to write tests that verify complex test output. To
write a snapshot test, make a `testhelpers.Snapshotter` at the beginning of
the test, set up a call to `Verify()` and call `Snapshot(name, value)`
whenever you want to test a complex output. For example:
```go
func TestWithComplexOutput(t *testing.T) {
    snapshotter := snapshotter.New(t)
    defer snapshotter.Verify()

    output := DoSomethingComplicated()
    snapshotter.Snapshot("complicated", output)
}
```
After writing the test, you can run it using `go test .`. However, the test
will fail, because the snapshot isn't there yet, with an error like
```
--- FAIL: TestSnapshotter (0.00s)
    snapshotter.go:89: error reading snapshots: open testdata/TestSnapshotter.snapshots.json: no such file or directory
```
To generate the snapshots, run `go test . -rewriteSnapshots`. This generates
a set of snapshots files in the testdata directory that should be added to git.
Then, the tests will pass. If it at some point a regression causes the test
output to break, the snapshotter will catch the changes and fail:
```
--- FAIL: TestSnapshotter (0.00s)
    snapshotter.go:116: snapshot second differs:
         [
          2,
          {
        -  Foo: "Bar",
        +  Foo: "Foo",
          },
         ]
```
Happy testing!


