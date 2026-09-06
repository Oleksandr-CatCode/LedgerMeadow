import { useSignIn, useSignUp } from '@clerk/react'
import { useState, type FormEvent } from 'react'
import { Icon } from '../../components/ui/Icon'
import { configureSessionPersistence } from './sessionPreference'

export type AuthMode = 'sign-in' | 'sign-up'

type AuthStep =
  | 'start'
  | 'sign-up-code'
  | 'mfa-email'
  | 'mfa-phone'
  | 'mfa-totp'
  | 'mfa-backup'
  | 'forgot-email'
  | 'forgot-code'
  | 'forgot-password'

type OAuthStrategy = 'oauth_google' | 'oauth_apple'

type AuthFlowProps = {
  mode: AuthMode
  onModeChange: (mode: AuthMode) => void
}

function errorMessage(error: unknown, fallback: string) {
  if (!error || typeof error !== 'object') return fallback

  if ('longMessage' in error && typeof error.longMessage === 'string') {
    return error.longMessage
  }
  if ('message' in error && typeof error.message === 'string') {
    return error.message
  }
  return fallback
}

function AuthHeading({ mode }: { mode: AuthMode }) {
  return (
    <div className="flex flex-col gap-[7px]">
      <h2 className="m-0 font-display text-[27px] leading-10 font-medium tracking-[-0.01em]">
        {mode === 'sign-in' ? 'Welcome back' : 'Create your account'}
      </h2>
      <p className="m-0 text-[13.5px] leading-5 text-muted">
        {mode === 'sign-in'
          ? 'Sign in to pick up where you left off.'
          : 'Set up a ledger in about two minutes.'}
      </p>
    </div>
  )
}

function SocialButton({
  disabled,
  icon,
  label,
  onClick,
}: {
  disabled: boolean
  icon: string
  label: string
  onClick: () => void
}) {
  return (
    <button
      className="auth-social-button"
      disabled={disabled}
      onClick={onClick}
      type="button"
    >
      <Icon className="text-base" name={icon} />
      {label}
    </button>
  )
}

function PasswordField({
  autoComplete,
  label = 'Password',
  onChange,
  placeholder,
  value,
}: {
  autoComplete: 'current-password' | 'new-password'
  label?: string
  onChange: (value: string) => void
  placeholder: string
  value: string
}) {
  const [visible, setVisible] = useState(false)

  return (
    <label className="auth-field">
      {label ? (
        <span className="flex items-baseline justify-between text-[12.5px] text-[#4a4844]">
          {label}
        </span>
      ) : null}
      <span className="relative block">
        <input
          autoComplete={autoComplete}
          className="auth-input pr-10"
          onChange={(event) => onChange(event.target.value)}
          placeholder={placeholder}
          required
          type={visible ? 'text' : 'password'}
          value={value}
        />
        <button
          aria-label={visible ? 'Hide password' : 'Show password'}
          className="absolute top-1/2 right-1 -translate-y-1/2 cursor-pointer rounded-md border-0 bg-transparent px-[7px] py-[5px] text-base text-faint transition-colors hover:bg-hover hover:text-ink"
          onClick={() => setVisible((current) => !current)}
          type="button"
        >
          <Icon name={visible ? 'eye-slash' : 'eye'} />
        </button>
      </span>
    </label>
  )
}

export function AuthFlow({ mode, onModeChange }: AuthFlowProps) {
  const {
    errors: signInErrors,
    fetchStatus: signInFetchStatus,
    signIn,
  } = useSignIn()
  const {
    errors: signUpErrors,
    fetchStatus: signUpFetchStatus,
    signUp,
  } = useSignUp()
  const [step, setStep] = useState<AuthStep>('start')
  const [fullName, setFullName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [code, setCode] = useState('')
  const [remember, setRemember] = useState(false)
  const [legalAccepted, setLegalAccepted] = useState(false)
  const [localError, setLocalError] = useState<string | null>(null)

  const busy = signInFetchStatus === 'fetching' || signUpFetchStatus === 'fetching'

  async function finalizeSignIn() {
    const { error } = await signIn.finalize({
      navigate: async ({ decorateUrl }) => {
        window.location.assign(decorateUrl('/'))
      },
    })
    if (error) setLocalError(errorMessage(error, 'Unable to finish signing in.'))
  }

  async function finalizeSignUp() {
    const { error } = await signUp.finalize({
      navigate: async ({ decorateUrl }) => {
        window.location.assign(decorateUrl('/'))
      },
    })
    if (error) setLocalError(errorMessage(error, 'Unable to finish creating your account.'))
  }

  async function prepareSecondFactor(forClientTrust: boolean) {
    const factors = signIn.supportedSecondFactors

    if (!forClientTrust && factors.some((factor) => factor.strategy === 'totp')) {
      setStep('mfa-totp')
      return
    }

    if (factors.some((factor) => factor.strategy === 'email_code')) {
      const { error } = await signIn.mfa.sendEmailCode()
      if (error) {
        setLocalError(errorMessage(error, 'Unable to send the verification code.'))
        return
      }
      setStep('mfa-email')
      return
    }

    if (factors.some((factor) => factor.strategy === 'phone_code')) {
      const { error } = await signIn.mfa.sendPhoneCode()
      if (error) {
        setLocalError(errorMessage(error, 'Unable to send the verification code.'))
        return
      }
      setStep('mfa-phone')
      return
    }

    if (factors.some((factor) => factor.strategy === 'backup_code')) {
      setStep('mfa-backup')
      return
    }

    setLocalError('This account requires an authentication method that is not available on this page.')
  }

  async function completeSignInStatus() {
    if (signIn.status === 'complete') {
      await finalizeSignIn()
      return
    }
    if (signIn.status === 'needs_client_trust') {
      await prepareSecondFactor(true)
      return
    }
    if (signIn.status === 'needs_second_factor') {
      await prepareSecondFactor(false)
      return
    }
    if (signIn.status === 'needs_new_password') {
      setStep('forgot-password')
      return
    }

    setLocalError('Clerk needs another authentication step before this sign-in can be completed.')
  }

  async function startOAuth(strategy: OAuthStrategy) {
    setLocalError(null)
    configureSessionPersistence(mode === 'sign-in' ? remember : true)
    const authResource = mode === 'sign-in' ? signIn : signUp
    const { error } = await authResource.sso({
      redirectCallbackUrl: '/sso-callback',
      redirectUrl: '/',
      strategy,
    })
    if (error) {
      setLocalError(errorMessage(error, `Unable to connect with ${strategy === 'oauth_google' ? 'Google' : 'Apple'}.`))
    }
  }

  async function submitCredentials(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setLocalError(null)

    const emailAddress = email.trim()
    if (!emailAddress || !password) return

    if (mode === 'sign-in') {
      const { error } = await signIn.password({ emailAddress, password })
      if (error) {
        setLocalError(errorMessage(error, 'Email or password is incorrect.'))
        return
      }
      configureSessionPersistence(remember)
      await completeSignInStatus()
      return
    }

    if (!legalAccepted) {
      setLocalError('Accept the Terms and Privacy Policy to create your account.')
      return
    }
    if (password.length < 10 || !/\d/.test(password)) {
      setLocalError('Use at least 10 characters with one number.')
      return
    }

    const normalizedFullName = fullName.trim()
    if (!normalizedFullName) {
      setLocalError('Enter your full name.')
      return
    }

    const { error } = await signUp.password({
      emailAddress,
      legalAccepted,
      password,
      unsafeMetadata: { full_name: normalizedFullName },
    })
    if (error) {
      setLocalError(errorMessage(error, 'Unable to create your account.'))
      return
    }

    if (signUp.status === 'complete') {
      configureSessionPersistence(true)
      await finalizeSignUp()
      return
    }

    if (signUp.unverifiedFields.includes('email_address')) {
      const { error: sendError } = await signUp.verifications.sendEmailCode()
      if (sendError) {
        setLocalError(errorMessage(sendError, 'Unable to send the verification code.'))
        return
      }
      setCode('')
      setStep('sign-up-code')
      return
    }

    setLocalError(`Your Clerk configuration requires: ${signUp.missingFields.join(', ') || 'an additional sign-up step'}.`)
  }

  async function verifySignUpCode(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setLocalError(null)
    const { error } = await signUp.verifications.verifyEmailCode({ code })
    if (error) {
      setLocalError(errorMessage(error, 'The verification code is incorrect.'))
      return
    }
    if (signUp.status === 'complete') {
      configureSessionPersistence(true)
      await finalizeSignUp()
      return
    }
    setLocalError(`Your Clerk configuration requires: ${signUp.missingFields.join(', ') || 'an additional sign-up step'}.`)
  }

  async function verifySecondFactor(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setLocalError(null)

    const result =
      step === 'mfa-email'
        ? await signIn.mfa.verifyEmailCode({ code })
        : step === 'mfa-phone'
          ? await signIn.mfa.verifyPhoneCode({ code })
          : step === 'mfa-totp'
            ? await signIn.mfa.verifyTOTP({ code })
            : await signIn.mfa.verifyBackupCode({ code })

    if (result.error) {
      setLocalError(errorMessage(result.error, 'The verification code is incorrect.'))
      return
    }
    await completeSignInStatus()
  }

  async function sendResetCode(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setLocalError(null)
    await signIn.reset()

    const { error: createError } = await signIn.create({ identifier: email.trim() })
    if (createError) {
      setLocalError(errorMessage(createError, 'Unable to find that account.'))
      return
    }
    const { error: sendError } = await signIn.resetPasswordEmailCode.sendCode()
    if (sendError) {
      setLocalError(errorMessage(sendError, 'Unable to send the reset code.'))
      return
    }
    setCode('')
    setStep('forgot-code')
  }

  async function verifyResetCode(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setLocalError(null)
    const { error } = await signIn.resetPasswordEmailCode.verifyCode({ code })
    if (error) {
      setLocalError(errorMessage(error, 'The reset code is incorrect.'))
      return
    }
    setStep('forgot-password')
  }

  async function submitNewPassword(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setLocalError(null)
    const { error } = await signIn.resetPasswordEmailCode.submitPassword({
      password: newPassword,
    })
    if (error) {
      setLocalError(errorMessage(error, 'Unable to update your password.'))
      return
    }
    configureSessionPersistence(remember)
    await completeSignInStatus()
  }

  async function changeMode(nextMode: AuthMode) {
    if (nextMode === mode) return
    await Promise.all([signIn.reset(), signUp.reset()])
    setStep('start')
    setCode('')
    setLocalError(null)
    onModeChange(nextMode)
  }

  const hookError =
    signInErrors.global?.[0]?.longMessage ??
    signUpErrors.global?.[0]?.longMessage ??
    signInErrors.fields.identifier?.message ??
    signInErrors.fields.password?.message ??
    signUpErrors.fields.emailAddress?.message ??
    signUpErrors.fields.password?.message
  const displayedError = localError ?? hookError

  if (step !== 'start') {
    const verificationStep = step.startsWith('mfa-') || step === 'sign-up-code'

    return (
      <div className="flex flex-col gap-[26px]">
        <div className="flex flex-col gap-[7px]">
          <h2 className="m-0 font-display text-[27px] font-medium tracking-[-0.01em]">
            {step === 'forgot-email'
              ? 'Reset your password'
              : step === 'forgot-password'
                ? 'Choose a new password'
                : 'Check your inbox'}
          </h2>
          <p className="m-0 text-[13.5px] leading-[1.5] text-muted">
            {step === 'forgot-email'
              ? 'We will send a secure reset code to your email.'
              : step === 'forgot-password'
                ? 'Use at least 10 characters with one number.'
                : step === 'mfa-totp'
                  ? 'Enter the code from your authenticator app.'
                  : step === 'mfa-backup'
                    ? 'Enter one of your unused backup codes.'
                    : `Enter the verification code sent for ${email || 'your account'}.`}
          </p>
        </div>

        <form
          className="flex flex-col gap-[15px]"
          onSubmit={
            step === 'forgot-email'
              ? sendResetCode
              : step === 'forgot-code'
                ? verifyResetCode
                : step === 'forgot-password'
                  ? submitNewPassword
                  : step === 'sign-up-code'
                    ? verifySignUpCode
                    : verifySecondFactor
          }
        >
          {step === 'forgot-email' ? (
            <label className="auth-field">
              <span>Email</span>
              <input
                autoComplete="email"
                className="auth-input"
                onChange={(event) => setEmail(event.target.value)}
                placeholder="alex@example.com"
                required
                type="email"
                value={email}
              />
            </label>
          ) : step === 'forgot-password' ? (
            <PasswordField
              autoComplete="new-password"
              label="New password"
              onChange={setNewPassword}
              placeholder="Choose a password"
              value={newPassword}
            />
          ) : (
            <label className="auth-field">
              <span>{step === 'mfa-backup' ? 'Backup code' : 'Verification code'}</span>
              <input
                autoComplete="one-time-code"
                className="auth-input tracking-[0.16em]"
                inputMode={step === 'mfa-backup' ? 'text' : 'numeric'}
                onChange={(event) => setCode(event.target.value)}
                placeholder={step === 'mfa-backup' ? 'Enter backup code' : '000000'}
                required
                value={code}
              />
            </label>
          )}

          {displayedError ? <p className="auth-error">{displayedError}</p> : null}
          <button className="auth-submit" disabled={busy} type="submit">
            {busy
              ? 'Please wait…'
              : step === 'forgot-email'
                ? 'Send reset code'
                : step === 'forgot-password'
                  ? 'Update password'
                  : 'Verify code'}
          </button>
        </form>

        <button
          className="w-fit cursor-pointer border-0 bg-transparent p-0 text-[13px] text-link transition-colors hover:text-ink"
          onClick={() => {
            setStep('start')
            setCode('')
            setLocalError(null)
          }}
          type="button"
        >
          Back to sign in
        </button>
        {verificationStep && step === 'sign-up-code' ? (
          <div className="empty:hidden" id="clerk-captcha" />
        ) : null}
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-[26px]">
      <AuthHeading mode={mode} />

      <div className="flex flex-col gap-[9px]">
        <SocialButton
          disabled={busy}
          icon="google-logo"
          label="Continue with Google"
          onClick={() => void startOAuth('oauth_google')}
        />
        <SocialButton
          disabled={busy}
          icon="apple-logo"
          label="Continue with Apple"
          onClick={() => void startOAuth('oauth_apple')}
        />
      </div>

      <div className="flex items-center gap-3 text-[11px] leading-[14px] tracking-[0.09em] text-faint uppercase">
        <span className="h-px flex-1 bg-line" />
        or
        <span className="h-px flex-1 bg-line" />
      </div>

      <form className="flex flex-col gap-[15px]" onSubmit={submitCredentials}>
        {mode === 'sign-up' ? (
          <label className="auth-field">
            <span>Full name</span>
            <input
              autoComplete="name"
              className="auth-input"
              onChange={(event) => setFullName(event.target.value)}
              placeholder="Alex Moreau"
              required
              type="text"
              value={fullName}
            />
          </label>
        ) : null}

        <label className="auth-field">
          <span>Email</span>
          <input
            autoComplete="email"
            className="auth-input"
            onChange={(event) => setEmail(event.target.value)}
            placeholder="alex@example.com"
            required
            type="email"
            value={email}
          />
        </label>

        <div className="auth-field">
          <span className="flex items-baseline justify-between text-[12.5px] text-[#4a4844]">
            Password
            {mode === 'sign-in' ? (
              <button
                className="cursor-pointer border-0 bg-transparent p-0 text-[12.5px] text-link transition-colors hover:text-ink"
                onClick={() => {
                  setStep('forgot-email')
                  setLocalError(null)
                }}
                type="button"
              >
                Forgot?
              </button>
            ) : null}
          </span>
          <PasswordField
            autoComplete={mode === 'sign-in' ? 'current-password' : 'new-password'}
            label=""
            onChange={setPassword}
            placeholder={mode === 'sign-in' ? '••••••••••' : 'Choose a password'}
            value={password}
          />
          {mode === 'sign-up' ? (
            <span className="text-xs leading-[15px] text-faint">
              At least 10 characters, with one number.
            </span>
          ) : null}
        </div>

        <label className="flex cursor-pointer items-start gap-[9px] text-[12.5px] leading-[1.5] text-[#4a4844]">
          <input
            checked={mode === 'sign-in' ? remember : legalAccepted}
            className="mt-0.5 size-3.5 accent-ink"
            onChange={(event) =>
              mode === 'sign-in'
                ? setRemember(event.target.checked)
                : setLegalAccepted(event.target.checked)
            }
            type="checkbox"
          />
          <span>
            {mode === 'sign-in'
              ? 'Keep me signed in on this device.'
              : 'I agree to the Terms and Privacy Policy.'}
          </span>
        </label>

        {displayedError ? <p className="auth-error">{displayedError}</p> : null}
        <div className="empty:hidden" id="clerk-captcha" />
        <button className="auth-submit mt-[3px]" disabled={busy} type="submit">
          {busy ? 'Please wait…' : mode === 'sign-in' ? 'Sign in' : 'Create account'}
        </button>
      </form>

      <p className="m-0 text-[13px] leading-[17px] text-muted">
        {mode === 'sign-in' ? 'New to LedgerMeadow?' : 'Already have an account?'}{' '}
        <button
          className="cursor-pointer border-0 bg-transparent p-0 text-link transition-colors hover:text-ink"
          onClick={() => void changeMode(mode === 'sign-in' ? 'sign-up' : 'sign-in')}
          type="button"
        >
          {mode === 'sign-in' ? 'Create an account' : 'Sign in'}
        </button>
      </p>
      <p className="m-0 text-[11.5px] leading-[1.6] text-faint">
        Sign-in is managed by Clerk. LedgerMeadow does not move money on your behalf.
      </p>
    </div>
  )
}
