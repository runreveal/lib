# lib

This repository contains a set of utility libraries which we find useful.

No guarantees around stability are provided at this time.

# await

A useful library for awaiting some synchronous tasks which accept a cancellable
context to either error or return cleanly upon context cancellation.

Similar to errgroup, but goroutines are not started until Run is called.

# loader

Loader was built to be able to load configuration for slices where the elements
satisfy the same interface, but the underlying implementation is different and
requires different configuration.  See the tests for examples.
