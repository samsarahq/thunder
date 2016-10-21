Thunder is a GraphQL server for Go with support for automatic live-updates.

## Getting Started & Documentation

To get started with Thunder, the [example](example/) is a good
starting point. Basic documentation is available online in
[godoc](http://godoc.org/github.com/samsarahq/thunder).

The source code in this repository is organized as follows:

- The root directory contains Thunder's core dependency-tracking and
  live-update mechanism.
- The livesql/ directory contains a Thunder driver for MySQL.
- The sqlgen/ directory contains a lightweight SQL query generator used by
  livesql/.
- The graphql/ directory contains Thunder's GraphQL parser and executor.
- The example/ directory contains a basic Thunder application.
- The experimental/ directory contains in-progress Thunder drivers for Postgres
  and Redis.

## Status

This repository is still under development, and there will likely be breaking
changes to the API until Thunder's first stable release.
