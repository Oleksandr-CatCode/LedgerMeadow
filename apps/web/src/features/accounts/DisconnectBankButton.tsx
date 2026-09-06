import { useAuth } from '@clerk/react'
import { useState } from 'react'
import { authenticatedOptions } from '../../api/config'
import { disconnectBankConnection } from '../../api/generated/sdk.gen'

type DisconnectBankButtonProps = {
  connectionId: string
  institutionName: string
  onDisconnected: () => Promise<void>
}

export function DisconnectBankButton({
  connectionId,
  institutionName,
  onDisconnected,
}: DisconnectBankButtonProps) {
  const { getToken } = useAuth()
  const [status, setStatus] = useState<'idle' | 'disconnecting' | 'error'>(
    'idle',
  )

  const disconnect = async () => {
    if (
      !window.confirm(
        `Disconnect “${institutionName}”? Future updates will stop and imported transaction history will be preserved.`,
      )
    ) {
      return
    }
    setStatus('disconnecting')
    const result = await disconnectBankConnection({
      ...authenticatedOptions(getToken),
      path: { id: connectionId },
    })
    if (result.error) {
      setStatus('error')
      return
    }
    setStatus('idle')
    await onDisconnected()
  }

  return (
    <div className="flex flex-col items-end gap-1">
      <button
        type="button"
        className="button-ghost text-[11.5px] text-danger"
        disabled={status === 'disconnecting'}
        onClick={() => void disconnect()}
      >
        {status === 'disconnecting' ? 'Disconnecting…' : 'Disconnect'}
      </button>
      {status === 'error' && (
        <span className="text-right text-[11.5px] text-danger" role="alert">
          The bank could not be disconnected.
        </span>
      )}
    </div>
  )
}
