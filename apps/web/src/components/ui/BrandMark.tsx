type BrandMarkProps = {
  className?: string
}

export function BrandMark({ className = 'size-5' }: BrandMarkProps) {
  return (
    <svg
      aria-hidden="true"
      className={className}
      viewBox="0 0 64 64"
      fill="none"
      stroke="currentColor"
      strokeWidth="5"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M14 12V52H52M30 36C30 20 39 12 54 12C54 27 46 36 30 36ZM30 36L45 21" />
    </svg>
  )
}
