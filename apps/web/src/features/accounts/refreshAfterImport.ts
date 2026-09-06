const refreshDelaysMs = [750, 1500, 3000]

type Sleep = (milliseconds: number) => Promise<void>

const sleep: Sleep = (milliseconds) =>
  new Promise((resolve) => window.setTimeout(resolve, milliseconds))

export async function refreshAfterImport(
  refresh: () => Promise<void>,
  wait: Sleep = sleep,
) {
  for (const delay of refreshDelaysMs) {
    await wait(delay)
    try {
      await refresh()
    } catch {
      return
    }
  }
}
