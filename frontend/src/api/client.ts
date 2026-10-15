const apiBaseUrl = import.meta.env.VITE_API_BASE_URL || '/api'

type HealthResponse = {
  data: {
    status: string
  }
}

export async function getHealth(signal?: AbortSignal): Promise<string> {
  const response = await fetch(`${apiBaseUrl}/healthz`, { signal })
  if (!response.ok) {
    throw new Error(`Health request failed (${response.status})`)
  }

  const body: HealthResponse = await response.json()
  if (body?.data?.status !== 'ok') {
    throw new Error('Unexpected health response')
  }
  return body.data.status
}
