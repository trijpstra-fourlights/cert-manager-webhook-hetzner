# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.5.0] - 2025-12-09

### Changed
- **BREAKING**: Updated GitHub Container Registry (GHCR.io) publishing to use `ghcr.io/trijpstra-fourlights/cert-manager-webhook-hetzner` instead of the original owner's registry
- Updated Go module path from `github.com/vadimkim/cert-manager-webhook-hetzner` to `github.com/trijpstra-fourlights/cert-manager-webhook-hetzner`
- Updated Helm chart version from 1.4.0 to 1.5.0

### Added
- Added manual workflow trigger (`workflow_dispatch`) to allow manual deployment through GitHub Actions
- Enhanced GitHub Actions workflow with manual deployment capability
- **Enhanced automatic zone detection**: Implemented intelligent zone detection that automatically searches for the correct Hetzner DNS zone when `zoneName` is not explicitly provided in the configuration
- **Parent domain traversal**: Added sophisticated zone lookup that iterates through parent domains (e.g., sub.domain.com → domain.com) to find the correct registered Hetzner zone
- **Improved configuration flexibility**: Zone detection now uses `ChallengeRequest.ResolvedZone` to automatically determine the appropriate zone, reducing configuration complexity

### Removed
- Removed Docker Hub publishing to focus exclusively on GitHub Container Registry (GHCR.io)
- Removed Docker Hub authentication step from CI/CD pipeline

### Migration Notes
- Users pulling images should now use `ghcr.io/trijpstra-fourlights/cert-manager-webhook-hetzner` instead of `ghcr.io/vadimkim/cert-manager-webhook-hetzner`
- Go modules imported from this repository will need to update their imports to use the new module path
- **Zone configuration is now optional**: The `zoneName` parameter in the webhook configuration is no longer required. If not provided, the webhook will automatically detect the correct zone by searching parent domains
- **Backwards compatibility**: Existing configurations with explicit `zoneName` will continue to work unchanged
- The webhook functionality and DNS challenge solving remain identical - only the publishing registry and configuration flexibility have been enhanced

## [1.4.0] - Previous Version
- This was the last version published by the original repository maintainer
- For detailed changes in this version, please refer to the original repository's changelog

---

**Note**: This repository was forked from the original [vadimkim/cert-manager-webhook-hetzner](https://github.com/vadimkim/cert-manager-webhook-hetzner) repository. This changelog documents changes made since the fork.