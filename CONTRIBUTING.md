# Contributing

Keep the web application, API, and financial engine in this monorepo. Open a branch and pull request for changes; protect the default branch in your fork or deployment workflow. Contributions use the repository's PolyForm Noncommercial license.

Run the checks from the root README. A change to `contracts/financial-engine` affects Go and Rust; rebuild/test both consumers and regenerate Go protobuf clients. A change to the OpenAPI contract affects the API and generated web client; check both.

Do not commit real financial samples, customer identifiers, provider payloads, local environment files, credentials, keys, screenshots, logs, or build output. Financial fixtures must be authored from scratch and clearly documented as synthetic. Credential tests use temporary generated values or deliberately invalid placeholders and must not log them.

Preserve authentication, SQL ownership scopes, input bounds, encrypted credential handling, generic errors, and private service boundaries. A disclosure-related change needs the smallest negative test proving sensitive content stays out of the response, log, redirect, or committed file.

After staging, run `python3 scripts/check_publication.py`. A clean current directory is insufficient: inspect all history before publishing a branch that may have contained private material.
