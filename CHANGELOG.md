# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Safe, deterministic declaration reordering compatible with `funcorder` v0.6.0 rule semantics.
- Exact-source movement that preserves function-body comments and directives.
- Conservative barriers for unattached non-whitespace trivia.
- stdin/stdout formatter mode, in-place writing, and CI check mode.
- Regression, idempotency, CRLF, anchor-order, fuzz-seed, and batch-safety tests.
