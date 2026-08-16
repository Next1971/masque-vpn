# Changelog

All notable changes to MASQUE VPN are documented here.

## [Unreleased]

### Documentation

- Added server deployment overview, security policy, contribution guidance, and issue templates.

## [v1.2] - 2026-08-16

### Fixed

- **Android multi-device:** the Android client now uses the address assigned by
  the server (via a two-phase connect) instead of a hardcoded TUN address, so
  multiple Android devices can be connected at the same time. Previously the
  second device connected but received no traffic.
- **Android TV:** "Import from file" no longer freezes the app on TVs without a
  file manager; it detects the missing handler and guides the user to paste-text
  import.

### Added

- **Android TV build** — a dedicated `tv` product flavor (application id
  `com.next1971.masque.tv`) with a leanback launcher, D-pad UI, and paste-text
  profile import. Installs side by side with the phone build.

### Known limitations

- Importing a profile from a file on Android TV is not supported yet (many TVs
  have no file manager). Use paste-text import instead.

### Documentation

- Added [docs/CLIENTS.md](docs/CLIENTS.md): how to issue one config bundle per
  device and add more devices reusing the same CA.

## [v1.1] - 2026-08-16

### Fixed

- **Multi-client routing (server):** a single TUN reader now demultiplexes
  return traffic to the correct client by destination IP, fixing concurrent
  connections from multiple clients.

## [v1.0] - 2026-08-14

### Added

- Initial public MASQUE VPN MVP release.
- Server, Windows client, and Android application sources.
- QUIC + HTTP/3 CONNECT-IP transport with mutual TLS, as described in the project README.

### Notes

- This release is experimental and has not received an independent security audit.
