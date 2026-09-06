import { UnavailableState } from '../../components/ui/AsyncState'

type UnavailableViewProps = {
  title: string
  description: string
  detail: string
}

export function UnavailableView({
  title,
  description,
  detail,
}: UnavailableViewProps) {
  return (
    <div>
      <h1 className="page-title">{title}</h1>
      <p className="mt-1.5 mb-[26px] text-sm text-muted">{description}</p>
      <UnavailableState feature={title} detail={detail} />
    </div>
  )
}
