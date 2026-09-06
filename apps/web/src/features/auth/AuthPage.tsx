import { useState } from 'react'
import { BrandMark } from '../../components/ui/BrandMark'
import { AuthFlow, type AuthMode } from './AuthFlow'

function LedgerArtwork() {
  return (
    <section className="auth-artwork relative flex min-w-0 flex-1 flex-col justify-between overflow-hidden bg-ink px-[46px] pt-10 pb-11 text-canvas">
      <div className="auth-glow auth-glow-brass" />
      <div className="auth-glow auth-glow-blue" />
      <div className="auth-glow auth-glow-cream" />

      <svg
        aria-hidden="true"
        className="absolute inset-0 size-full"
        preserveAspectRatio="none"
        viewBox="0 0 800 400"
        fill="none"
      >
        <path
          className="auth-ledger-line auth-ledger-line-cream"
          d="M-20 300 C 90 300, 130 210, 220 214 S 350 268, 430 190 S 560 96, 660 128 S 780 60, 830 66"
          stroke="#e7e0ce"
          strokeWidth="1.5"
          strokeLinecap="round"
        />
        <path
          className="auth-ledger-line auth-ledger-line-brass"
          d="M-20 350 C 100 340, 160 296, 250 300 S 380 336, 470 262 S 600 208, 690 226 S 790 176, 830 168"
          stroke="#8a7448"
          strokeWidth="1.5"
          strokeLinecap="round"
        />
        <path
          className="auth-ledger-line auth-ledger-line-blue"
          d="M-20 240 C 120 244, 190 150, 300 162 S 430 214, 520 124 S 650 40, 830 24"
          stroke="#3d5c70"
          strokeWidth="1.2"
          strokeLinecap="round"
        />
      </svg>

      <div className="relative flex items-center gap-3">
        <BrandMark className="size-[22px]" />
        <span className="font-display text-[15px] font-medium tracking-[0.16em]">
          LedgerMeadow
        </span>
      </div>

      <div className="relative flex max-w-[460px] flex-col gap-[18px]">
        <h1 className="m-0 font-display text-[42px] leading-[1.14] font-normal tracking-[-0.01em] text-pretty">
          Every dollar, in one clear line.
        </h1>
        <p className="m-0 max-w-[40ch] text-[15px] leading-[1.6] text-[rgba(244,243,241,.72)]">
          Accounts, budgets, bills and goals kept in a single ledger — so the
          month ahead is never a guess.
        </p>
      </div>

      <div className="relative flex gap-[30px] text-[12.5px] text-[rgba(244,243,241,.55)]">
        <span>Bank-level encryption</span>
        <span>Read-only connections</span>
        <span>No data resale</span>
      </div>
    </section>
  )
}

function AuthTabs({ mode, onChange }: { mode: AuthMode; onChange: (mode: AuthMode) => void }) {
  return (
    <div
      aria-label="Authentication mode"
      className="flex gap-1 rounded-[9px] border border-line bg-sidebar p-[3px]"
      role="tablist"
    >
      <button
        aria-selected={mode === 'sign-in'}
        className={`auth-tab ${mode === 'sign-in' ? 'auth-tab-active' : ''}`}
        onClick={() => onChange('sign-in')}
        role="tab"
        type="button"
      >
        Sign in
      </button>
      <button
        aria-selected={mode === 'sign-up'}
        className={`auth-tab ${mode === 'sign-up' ? 'auth-tab-active' : ''}`}
        onClick={() => onChange('sign-up')}
        role="tab"
        type="button"
      >
        Create account
      </button>
    </div>
  )
}

export function AuthPage() {
  const [mode, setMode] = useState<AuthMode>('sign-in')

  function changeMode(nextMode: AuthMode) {
    if (nextMode === mode) return

    window.history.replaceState(null, '', window.location.pathname)
    setMode(nextMode)
  }

  return (
    <main className="flex min-h-screen bg-canvas">
      <LedgerArtwork />
      <section className="flex w-[520px] shrink-0 flex-col justify-center bg-canvas px-14 py-12">
        <div className="mx-auto flex w-full max-w-[380px] flex-col gap-[26px]">
          <AuthTabs mode={mode} onChange={changeMode} />

          <AuthFlow key={mode} mode={mode} onModeChange={changeMode} />
        </div>
      </section>
    </main>
  )
}
