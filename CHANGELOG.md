## Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]
### Added
- Rarimo module configuration
- Rarimo withdraw processor
- Rarimo vault secrets functionality

### Changed
- EVM config contract addresses in the example to the actual one
- bump `near-go` version
- Near withdraw processor moved to the bridgers

### Fixed
- Vault config nil pointer in the evm and solana bridgers

## [v1.0.0] - 2023-10-23
### Under the hood changes
- Initiated project

[Unreleased]: https://github.com/rarimo/relayer-svc/compare/v1.0.0...HEAD
[v1.0.0]: https://github.com/rarimo/relayer-svc/releases/tag/v1.0.0
