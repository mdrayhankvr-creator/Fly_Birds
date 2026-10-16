import { useCallback } from 'react'
import { ApiError } from '../api/client'
import { useAuth } from '../auth/AuthProvider'

export const peopleField = 'mt-1 min-w-0 w-full max-w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-slate-900 focus:border-teal-700 focus:outline-none'

export function usePeopleApi() {
  const { accessToken, renew, expire } = useAuth()
  return useCallback(async <T,>(request: (token: string) => Promise<T>): Promise<T> => {
    if (!accessToken) throw new ApiError(401, 'unauthorized')
    try { return await request(accessToken) }
    catch (cause) {
      if (cause instanceof ApiError && cause.status === 401) {
        try { return await request(await renew()) }
        catch (retryCause) { if (retryCause instanceof ApiError && retryCause.status === 401) expire(); throw retryCause }
      }
      throw cause
    }
  }, [accessToken, renew, expire])
}
