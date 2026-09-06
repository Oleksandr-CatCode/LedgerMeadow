import { useState } from 'react'
import type { Account } from '../../api/generated/types.gen'
import { BrandMark } from '../../components/ui/BrandMark'
import { Icon } from '../../components/ui/Icon'
import { Money } from '../../components/ui/Money'

type CardsViewProps = {
  accounts: Account[]
}

export function CardsView({ accounts }: CardsViewProps) {
  const cardAccounts = accounts.filter(isCardAccount)
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const selected =
    cardAccounts.find((account) => account.id === selectedId) ??
    cardAccounts[0] ??
    null

  return (
    <div>
      <div className="flex items-end justify-between gap-5">
        <div>
          <h1 className="page-title">Cards</h1>
          <p className="mt-1 text-[13.5px] text-muted">
            Manage how your money can be spent.
          </p>
        </div>
        <button type="button" className="button-primary" disabled>
          <Icon name="plus" className="text-sm" /> New card
        </button>
      </div>

      <section className="mt-5 flex max-w-[1180px] items-start gap-3 rounded-[10px] border border-[#d9cda9] bg-[#f5f0e3] px-4 py-3.5">
        <Icon
          name="info"
          className="mt-0.5 shrink-0 text-base text-[#8a7448]"
        />
        <div>
          <div className="text-[13.5px] font-medium text-ink">
            Preview — Cards are under development
          </div>
          <p className="mt-1 mb-0 text-[12.5px] leading-[1.5] text-muted">
            This preview uses information from your connected card accounts.
            LedgerMeadow cannot issue cards, reveal card details, freeze cards, or set
            spending limits yet.
          </p>
        </div>
      </section>

      <div className="mt-[22px] grid max-w-[1180px] grid-cols-[minmax(330px,380px)_minmax(0,1fr)] items-start gap-[34px]">
        <div>
          <CardPreview account={selected} />

          <div className="mt-3.5 flex gap-2">
            <button type="button" className="button-primary flex-1" disabled>
              Card details
            </button>
            <button
              type="button"
              className="button-secondary flex-1"
              disabled
            >
              Freeze
            </button>
            <button
              type="button"
              className="button-secondary flex-1"
              disabled
            >
              Limits
            </button>
          </div>

          <AccountSummary account={selected} />
        </div>

        <div className="flex min-w-0 flex-col gap-[22px]">
          {cardAccounts.length > 0 ? (
            <ConnectedCardAccounts
              accounts={cardAccounts}
              selectedId={selected?.id ?? null}
              onSelect={setSelectedId}
            />
          ) : (
            <section>
              <div className="eyebrow mb-2">Connected card accounts</div>
              <div className="rounded-[10px] border border-dashed border-line px-[18px] py-5 text-center">
                <Icon
                  name="credit-card"
                  className="text-xl text-faint"
                />
                <div className="mt-2 text-sm font-medium">
                  No card account information yet
                </div>
                <p className="mx-auto mt-1 mb-0 max-w-[360px] text-[12.5px] leading-[1.5] text-muted">
                  Credit-card accounts imported through Accounts will appear
                  here as a read-only preview.
                </p>
              </div>
            </section>
          )}

          <PreviewFeatureGroup
            title="Subscription cards"
            features={[
              {
                name: 'Dedicated subscription cards',
                detail: 'Control a recurring payment with its own card.',
              },
            ]}
          />
          <PreviewFeatureGroup
            title="Custom cards"
            features={[
              {
                name: 'Spending cards',
                detail:
                  'Create a card for Travel, Shopping, or another purpose.',
              },
              {
                name: 'Household cards',
                detail: 'Share controlled spending with your household.',
              },
            ]}
          />
        </div>
      </div>
    </div>
  )
}

function CardPreview({ account }: { account: Account | null }) {
  return (
    <div className="relative aspect-[1.586/1] overflow-hidden rounded-[14px] bg-[#1c1c1a] px-6 py-[22px] shadow-[0_6px_22px_rgba(25,25,24,.14)]">
      <div className="pointer-events-none absolute inset-[9px] rounded-lg border border-[rgba(138,116,72,.3)]" />
      <div className="pointer-events-none absolute inset-3 border-t border-[rgba(138,116,72,.16)]" />
      <BrandMark className="relative size-[26px] text-[#e8e4dc]" />

      <div className="absolute right-6 bottom-[22px] left-6">
        <div className="text-[8.5px] tracking-[0.16em] text-[#86817a] uppercase">
          LedgerMeadow card preview
        </div>
        <div className="mt-1 truncate text-[13px] font-[450] tracking-[0.2em] text-[#e8e4dc] uppercase">
          {account?.name ?? 'Future card'}
        </div>
        {account && (
          <div className="mt-3">
            <div className="text-[8.5px] tracking-[0.16em] text-[#86817a] uppercase">
              Connected balance
            </div>
            <Money
              amountMinor={account.balance_minor}
              currency={account.currency}
              className="mt-0.5 block text-sm text-[#e8e4dc] tabular-nums"
            />
          </div>
        )}
        <div className="mt-[18px] flex items-end justify-between gap-5">
          <span className="text-[15px] tracking-[0.2em] text-[#e8e4dc] tabular-nums">
            {account?.mask ? `•••• ${account.mask}` : '•••• PREVIEW'}
          </span>
          <span className="font-display text-xs tracking-[0.3em] text-[#e8e4dc]">
            LedgerMeadow
          </span>
        </div>
      </div>
    </div>
  )
}

function AccountSummary({ account }: { account: Account | null }) {
  return (
    <section className="panel mt-[18px] px-5 pt-[18px] pb-4">
      <div className="eyebrow">Connected account data</div>
      {account ? (
        <>
          <Money
            amountMinor={
              account.available_balance_minor ?? account.balance_minor
            }
            currency={account.currency}
            className="mt-1.5 block text-[32px] font-medium tracking-[-0.02em] tabular-nums"
          />
          <div className="mt-3.5 flex flex-col border-t border-line-soft">
            <SummaryRow label="Account balance">
              <Money
                amountMinor={account.balance_minor}
                currency={account.currency}
              />
            </SummaryRow>
            {account.available_balance_minor !== null &&
              account.available_balance_minor !== undefined && (
                <SummaryRow label="Available balance">
                  <Money
                    amountMinor={account.available_balance_minor}
                    currency={account.currency}
                  />
                </SummaryRow>
              )}
            <SummaryRow label="Account type">
              {formatAccountType(account)}
            </SummaryRow>
            <SummaryRow label="Data source">
              {account.provider === 'FILE_IMPORT' ? 'File import' : 'Plaid'}
            </SummaryRow>
            <SummaryRow label="Card status">
              <span className="inline-flex items-center gap-1.5 text-[#8a7448]">
                <span className="size-1.5 rounded-full bg-[#8a7448]" />
                Preview only
              </span>
            </SummaryRow>
          </div>
        </>
      ) : (
        <p className="mt-2 mb-0 text-[12.5px] leading-[1.5] text-muted">
          Connect or import a credit-card account to populate this preview with
          existing account information.
        </p>
      )}
      <p className="mt-3 border-t border-line-soft pt-3 text-[12.5px] leading-[1.5] text-muted">
        These balances come from your connected account. They do not represent
        an issued LedgerMeadow card or an enforceable spending limit.
      </p>
    </section>
  )
}

function SummaryRow({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="flex items-baseline justify-between gap-4 border-b border-line-soft py-[9px] text-[13.5px] last:border-b-0">
      <span className="text-[13px] text-muted">{label}</span>
      <span className="text-right tabular-nums">{children}</span>
    </div>
  )
}

function ConnectedCardAccounts({
  accounts,
  selectedId,
  onSelect,
}: {
  accounts: Account[]
  selectedId: string | null
  onSelect: (accountId: string) => void
}) {
  return (
    <section>
      <div className="eyebrow mb-2">Connected card accounts</div>
      <div className="flex flex-col gap-2">
        {accounts.map((account) => (
          <button
            key={account.id}
            type="button"
            className={`grid w-full cursor-pointer grid-cols-[1fr_auto] items-baseline gap-x-4 gap-y-[3px] rounded-[10px] border border-line px-[15px] py-[13px] text-left transition-colors hover:bg-soft ${selectedId === account.id ? 'bg-white shadow-[inset_2px_0_0_#3d5c70]' : 'bg-transparent'}`}
            onClick={() => onSelect(account.id)}
          >
            <span className="truncate text-sm font-medium">{account.name}</span>
            <Money
              amountMinor={
                account.available_balance_minor ?? account.balance_minor
              }
              currency={account.currency}
              className="text-sm tabular-nums"
            />
            <span className="text-[12.5px] text-faint tabular-nums">
              {account.mask
                ? `•••• ${account.mask}`
                : formatAccountType(account)}
            </span>
            <span className="text-[12.5px] text-faint">
              {account.available_balance_minor !== null &&
              account.available_balance_minor !== undefined
                ? 'available'
                : 'balance'}
            </span>
          </button>
        ))}
      </div>
    </section>
  )
}

function PreviewFeatureGroup({
  title,
  features,
}: {
  title: string
  features: Array<{ name: string; detail: string }>
}) {
  return (
    <section>
      <div className="eyebrow mb-2">{title}</div>
      <div className="flex flex-col gap-2">
        {features.map((feature) => (
          <div
            key={feature.name}
            className="grid grid-cols-[1fr_auto] gap-x-4 gap-y-[3px] rounded-[10px] border border-dashed border-line px-[15px] py-[13px]"
          >
            <span className="text-sm font-medium">{feature.name}</span>
            <span className="text-[12.5px] text-[#8a7448]">Coming soon</span>
            <span className="col-span-2 text-[12.5px] text-faint">
              {feature.detail}
            </span>
          </div>
        ))}
      </div>
    </section>
  )
}

function isCardAccount(account: Account) {
  const type = `${account.type} ${account.subtype ?? ''}`.toLowerCase()
  return account.type.toLowerCase() === 'credit' || type.includes('card')
}

function formatAccountType(account: Account) {
  return (account.subtype ?? account.type)
    .replaceAll('_', ' ')
    .replaceAll('-', ' ')
}
