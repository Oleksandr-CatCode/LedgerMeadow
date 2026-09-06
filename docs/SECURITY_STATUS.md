# Security and deployment status

This release publishes development source with a sanitized repository and additional disclosure protections. It is not an approval to deploy a service holding real customers' bank credentials or financial records.

## Controls checked during release preparation

- Central Clerk authentication on financial routes; authorization remains in Go. Sampled account, transaction, search, and household queries scope access in SQL, with referenced-object checks and database ownership constraints.
- Backend provider tokens are encrypted before persistence and excluded from client DTOs. Disconnect revokes upstream tokens before clearing stored credentials.
- Plaid webhooks validate algorithm, timestamp, signature, and body digest before durable deduplicated work is queued.
- Provider redirects are rejected so credential headers and request bodies cannot be forwarded to a different endpoint.
- Provider synchronization errors are mapped to stable application codes before persistence or display.
- API responses receive `no-store`, no-referrer, nosniff, and frame-denial headers, including denied requests. Request logs contain matched route templates, not raw paths or query strings. Migration failures use generic codes rather than raw database errors.
- The plaintext gRPC caller and server accept only literal loopback addresses. Message limits remain enforced. Engine ports are not exposed by the image definition.
- The local startup script isolates backend credentials from frontend and engine processes. Fonts and icon styles are bundled locally from pinned dependencies.
- Negative regression tests cover these disclosure boundaries; normal tests cover financial calculations and existing application behavior.

No live bank service, private database, real account records, or production credentials were used for release validation. Passing tests cannot prove the absence of every security defect.

## Work required before handling production bank data

- Authenticated encrypted transport for services on different hosts; a trusted shared host/network namespace is the current RPC boundary.
- Production secret management, contextual encryption binding, key versioning and rotation, and restore procedures.
- Complete rate limiting, request correlation/recovery, and negative caching of failed webhook key lookups.
- Server-enforced MFA/step-up for sensitive operations; frontend text or UI flow is not an enforcement boundary.
- Least-privilege database roles, row-level-security defense in depth, and append-only audit-role permissions verified in the actual deployment.
- A deployment-specific strict Content-Security-Policy compatible with configured identity and bank SDKs.
- Review OAuth continuation storage: the frontend currently uses session storage for short-lived browser-facing Plaid Link tokens. These are distinct from backend bank-access credentials, but persistent browser storage does not meet an in-memory-only policy.
- Household split acceptance: pending obligations must not be treated as participant-approved financial obligations.
- Dedicated authorization, abuse, concurrency, migration, and end-to-end testing against synthetic staging data, plus an independent security assessment.

Public source and private runtime configuration have separate lifecycles. Review any newly created logs, database exports, screenshots, and bundles before sharing them. Never attach real financial evidence to a public issue.

## Dependency audit scope

Release preparation updated the Go toolchain and vulnerable dependencies. Symbol-level `govulncheck` reports no reachable vulnerabilities, and no imported-package findings. The [OpenPGP advisory GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932) remains in an unused part of `golang.org/x/crypto`; that package is not imported or called by this application and has no fixed version in the advisory database. Re-evaluate the dependency graph before adding OpenPGP functionality. npm and RustSec audits report no known vulnerabilities at the release check; advisory databases change over time.
