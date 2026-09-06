import { useAuth } from '@clerk/react'
import { useState } from 'react'
import { authenticatedOptions } from '../../api/config'
import {
  importRbcCsv,
  previewRbcCsvImport,
} from '../../api/generated/sdk.gen'
import type { RbcCsvPreview } from '../../api/generated/types.gen'
import { Money } from '../../components/ui/Money'
import { parseMoneyInput } from '../../lib/money/formatMoney'
import { refreshAfterImport } from './refreshAfterImport'

type RbcCsvImportDialogProps = {
  onClose: () => void
  onImported: () => Promise<void>
}

type ImportSummary = {
  accountName: string
  importedCount: number
  duplicateCount: number
}

export function RbcCsvImportDialog({
  onClose,
  onImported,
}: RbcCsvImportDialogProps) {
  const { getToken } = useAuth()
  const [file, setFile] = useState<File | null>(null)
  const [preview, setPreview] = useState<RbcCsvPreview | null>(null)
  const [currentBalance, setCurrentBalance] = useState('')
  const [status, setStatus] = useState<'idle' | 'checking' | 'importing'>(
    'idle',
  )
  const [error, setError] = useState<string | null>(null)
  const [importSummary, setImportSummary] = useState<ImportSummary | null>(null)

  const importFile = async (
    selectedFile: File,
    selectedPreview: RbcCsvPreview,
    balanceMinor?: string,
  ) => {
    setStatus('importing')
    const result = await importRbcCsv({
      ...authenticatedOptions(getToken),
      body: {
        file: selectedFile,
        ...(balanceMinor === undefined
          ? {}
          : { current_balance_minor: balanceMinor }),
      },
    })
    if (result.error || !result.data) {
      setStatus('idle')
      if (selectedPreview.existing_account) {
        setFile(null)
        setPreview(null)
      }
      setError(
        result.error?.message || 'The RBC transactions could not be imported.',
      )
      return
    }
    setStatus('idle')
    setFile(null)
    setPreview(null)
    setImportSummary({
      accountName: selectedPreview.account_name,
      importedCount: result.data.imported_count,
      duplicateCount: result.data.duplicate_count,
    })
    await onImported()
    void refreshAfterImport(onImported)
  }

  const inspectFile = async (selectedFile: File) => {
    setStatus('checking')
    setError(null)
    const result = await previewRbcCsvImport({
      ...authenticatedOptions(getToken),
      body: { file: selectedFile },
    })
    if (result.error || !result.data) {
      setStatus('idle')
      setFile(null)
      setError(
        result.error?.message ||
          'This file is not a supported single-account RBC CSV.',
      )
      return
    }
    const nextPreview = result.data
    setPreview(nextPreview)
    if (nextPreview.existing_account) {
      await importFile(selectedFile, nextPreview)
      return
    }
    setStatus('idle')
  }

  const importNewAccount = async () => {
    if (!file || !preview || status !== 'idle') return
    const balanceMinor = parseMoneyInput(currentBalance)
    if (balanceMinor === null) {
      setError('Enter the current balance with no more than two decimals.')
      return
    }
    setError(null)
    await importFile(file, preview, balanceMinor)
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-[rgba(25,25,24,.26)] p-5"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget && status === 'idle') onClose()
      }}
    >
      <section
        role="dialog"
        aria-modal="true"
        aria-labelledby="rbc-csv-title"
        className="max-h-[90vh] w-full max-w-[560px] overflow-y-auto rounded-xl border border-line bg-white px-7 py-6 shadow-[0_12px_40px_rgba(25,25,24,.14)]"
      >
        <div className="flex items-start justify-between gap-5">
          <div>
            <div className="eyebrow">File import</div>
            <h2 id="rbc-csv-title" className="mt-2 mb-0 text-xl font-medium">
              Import RBC transactions
            </h2>
          </div>
          <button
            type="button"
            className="button-secondary"
            disabled={status !== 'idle'}
            onClick={onClose}
          >
            Close
          </button>
        </div>

        {status !== 'idle' ? (
          <div className="mt-6 rounded-lg bg-soft p-5">
            <div className="text-sm font-medium">
              {status === 'checking'
                ? 'Checking the RBC file…'
                : preview?.existing_account
                  ? `Importing into ${preview.account_name}…`
                  : 'Creating the account and importing transactions…'}
            </div>
            <p className="mt-2 mb-0 text-xs text-faint">
              The file is processed in memory and is not retained.
            </p>
          </div>
        ) : importSummary ? (
          <div className="mt-6">
            <div className="rounded-lg bg-soft p-5" role="status">
              <div className="text-sm font-medium">Data updated</div>
              <p className="mt-2 mb-0 text-sm text-muted">
                {importSummary.importedCount === 0
                  ? `No new transactions were found for ${importSummary.accountName}.`
                  : `${importSummary.importedCount} new ${
                      importSummary.importedCount === 1
                        ? 'transaction was'
                        : 'transactions were'
                    } added to ${importSummary.accountName}.`}
              </p>
              <p className="mt-2 mb-0 text-xs text-faint">
                {importSummary.duplicateCount}{' '}
                {importSummary.duplicateCount === 1
                  ? 'existing transaction was'
                  : 'existing transactions were'}{' '}
                detected and skipped.
              </p>
            </div>
            <div className="mt-5 flex gap-2">
              <button
                type="button"
                className="button-primary"
                onClick={() => setImportSummary(null)}
              >
                Upload another CSV
              </button>
              <button
                type="button"
                className="button-secondary"
                onClick={onClose}
              >
                Done
              </button>
            </div>
          </div>
        ) : !preview ? (
          <div className="mt-6">
            <p className="text-sm text-muted">
              Choose an RBC Visa or chequing CSV. LedgerMeadow will detect the account,
              currency, transactions, and previous imports automatically.
            </p>
            <input
              id="rbc-csv"
              className="sr-only"
              type="file"
              accept=".csv,text/csv"
              disabled={status !== 'idle'}
              onChange={(event) => {
                const selectedFile = event.target.files?.[0] ?? null
                setFile(selectedFile)
                setPreview(null)
                setImportSummary(null)
                setError(null)
                setCurrentBalance('')
                if (selectedFile) void inspectFile(selectedFile)
              }}
            />
            <label
              htmlFor="rbc-csv"
              className="mt-5 flex cursor-pointer flex-col items-center rounded-xl border-2 border-dashed border-line bg-soft px-6 py-8 text-center transition-colors hover:border-link hover:bg-white"
            >
              <i
                aria-hidden="true"
                className="ph-light ph-upload-simple text-3xl text-link"
              />
              <span className="mt-3 text-[15px] font-medium text-ink">
                Click here to choose your downloaded RBC CSV
              </span>
              <span className="mt-1 text-xs text-muted">
                In the file window, open Downloads and select the RBC .csv file
              </span>
            </label>
            <p className="mt-2 text-xs text-faint">
              Import starts automatically after selection. New accounts ask once
              for the current balance because RBC does not include it in this CSV.
            </p>
          </div>
        ) : (
          <div className="mt-6">
            <div className="mb-4 text-sm font-medium">
              New account detected: {preview.account_name}
            </div>
            <div className="grid grid-cols-2 gap-x-6 gap-y-4 rounded-lg bg-soft p-4 text-sm">
              <Summary label="Account">
                {preview.account_type === 'VISA' ? 'Visa' : 'Chequing'} ••••{' '}
                {preview.mask}
              </Summary>
              <Summary label="Currency">{preview.currency}</Summary>
              <Summary label="Transactions">
                {preview.transaction_count}
              </Summary>
              <Summary label="Date range">
                {preview.date_from} – {preview.date_to}
              </Summary>
              <Summary label="Positive rows">
                <Money
                  amountMinor={preview.positive_total_minor}
                  currency={preview.currency}
                />
              </Summary>
              <Summary label="Spending / charges">
                <Money
                  amountMinor={preview.outflow_total_minor}
                  currency={preview.currency}
                />
              </Summary>
            </div>

            <label className="field-label mt-5 block" htmlFor="rbc-current-balance">
              {preview.account_type === 'VISA'
                ? 'Amount currently owed'
                : 'Current account balance'}{' '}
              ({preview.currency})
            </label>
            <input
              id="rbc-current-balance"
              className="field-control mt-2 w-full"
              type="text"
              inputMode="decimal"
              placeholder="0.00"
              value={currentBalance}
              disabled={status !== 'idle'}
              onChange={(event) => setCurrentBalance(event.target.value)}
            />
            <p className="mt-3 text-xs text-faint">
              RBC CSV files do not include the live balance. For Visa, enter a
              positive amount owed; LedgerMeadow stores it as a liability.
            </p>
            <p className="mt-2 text-xs text-faint">
              Positive rows remain positive. Review card payments, refunds, and
              transfers after import so their categories reflect your intent.
            </p>
            <div className="mt-5 flex gap-2">
              <button
                type="button"
                className="button-primary"
                disabled={status !== 'idle'}
                onClick={() => void importNewAccount()}
              >
                Import transactions
              </button>
              <button
                type="button"
                className="button-secondary"
                disabled={status !== 'idle'}
                onClick={() => {
                  setPreview(null)
                  setFile(null)
                  setCurrentBalance('')
                  setError(null)
                }}
              >
                Choose another file
              </button>
            </div>
          </div>
        )}

        {error && (
          <p className="mt-4 text-sm text-danger" role="alert">
            {error}
          </p>
        )}
      </section>
    </div>
  )
}

function Summary({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div>
      <div className="text-xs text-faint">{label}</div>
      <div className="mt-1 tabular-nums">{children}</div>
    </div>
  )
}
