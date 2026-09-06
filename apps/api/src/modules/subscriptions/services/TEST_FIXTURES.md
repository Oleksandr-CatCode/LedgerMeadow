# Synthetic subscription test schedule

The subscription and payment schedule in `service_test.go` were authored from
scratch for the public release. The patterned UUID, merchant label, dates, and
amounts are invented test inputs; they do not represent a financial account,
subscription, or transaction history.

The fixture preserves a five-payment history so the test can check the response
history bound, normalization of signed amounts, and omission of internal linkage.
The fake engine returns summary values computed for these synthetic inputs.

Do not replace these inputs with records from a real account or bank statement.

Related merchant references in `subscriptions/repository/repository_integration_test.go`
and `planning/services/service_test.go` were replaced with synthetic labels as well.
The repository integration fixture uses the same invented payment amount, and the
planning test uses a fixed fictional calendar in the year 2000. These paths are
relative to `apps/api/src/modules`.

The Rust planning unit fixture in `apps/financial-engine/src/planning/mod.rs`
also uses the invented subscription label and amount with a year-2000 calendar.
Its expected totals were recalculated for that synthetic input. No production
calculation logic changed.
