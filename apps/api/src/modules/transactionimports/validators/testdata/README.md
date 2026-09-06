# Synthetic bank-import fixtures

Every inline CSV input in `../rbc_csv_test.go` was authored from scratch for
public testing. Account fields, descriptions, calendar dates, and amounts are
invented inputs. The card-like account field consists of sixteen zeros, and its
expected mask consists of four zeros. Chequing inputs use zero-filled account
fields, with one final digit varied only to test rejection of multiple accounts.

The inline tests cover signed amounts and masking, stable fingerprints for
identical rows, rejection of multiple accounts, and rejection of rows containing
both currency columns. They contain no copied bank records.

`rbc_chequing.csv` was also authored from scratch. Its account identifier, dates,
descriptions, and amounts are invented test inputs. No rows were copied from a
bank statement, financial account, or private dataset.

The file header follows the parser's expected RBC export format. Each data row
has one extra empty cell at the end. This intentionally exercises the parser's
trailing-empty-field compatibility behavior.

All fixture data in `services/service_test.go` and
`repository/repository_integration_test.go`, within the `transactionimports`
module, was authored anew: account names, masks, user identifiers, opaque
connection/account/batch identifiers, transaction fingerprints, descriptions,
dates, and amounts. Service and repository cases use zero masks and an invented
calendar in January 2000. Runtime suffixes isolate each test's generated rows.

The repository test starts with a synthetic 500-minor-unit card balance and then
imports one additional debit of 3 minor units. Its expected stored balance is
therefore -503 minor units; duplicate rows do not change that balance. Identity
reuse, token absence, transaction counts, and analysis-event checks remain intact.

The handler's valid-balance example is also synthetic. Empty input and signed
integer rejection cases are retained as format boundaries, not financial records.

Do not add real bank exports, account numbers, or card numbers. New fixtures
must be authored synthetically and described here. A variable or test name
containing "synthetic" is not evidence of a value's provenance.
