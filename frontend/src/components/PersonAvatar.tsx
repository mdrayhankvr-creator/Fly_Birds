import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useAuth } from '../auth/AuthProvider'
import { getMyStudent, getStudentPhoto } from '../api/client'
import { getStaffPhoto, getStaffProfile } from '../api/staffProfile'

type Kind = 'student' | 'staff'

export function usePersonPhoto(kind: Kind, id?: string, version?: string, tryWithoutVersion = false) {
  const { accessToken, user } = useAuth()
  const photo = useQuery({
    queryKey: ['avatar', user?.id, kind, id, version || 'unversioned'],
    enabled: !!accessToken && !!id && (!!version || tryWithoutVersion),
    queryFn: () => kind === 'student' ? getStudentPhoto(accessToken!, id!, version) : getStaffPhoto(accessToken!, id!, version || ''),
    retry: false,
    staleTime: version ? Infinity : 0,
  })
  const [url, setUrl] = useState('')
  useEffect(() => {
    if (!photo.data) { setUrl(''); return }
    const next = URL.createObjectURL(photo.data)
    setUrl(next)
    return () => URL.revokeObjectURL(next)
  }, [photo.data])
  return url
}

export function PersonAvatar({ name, kind, id, version, tryWithoutVersion = false, className = 'h-10 w-10 rounded-full text-sm', fallbackClassName = 'bg-[var(--brand-soft)] text-[var(--brand-ink)]' }: {
  name: string; kind?: Kind; id?: string; version?: string; tryWithoutVersion?: boolean; className?: string; fallbackClassName?: string
}) {
  const url = usePersonPhoto(kind || 'student', id, version, tryWithoutVersion)
  const letters = name.split(/[\s@.]+/).filter(Boolean).slice(0, 2).map(part => [...part][0]).join('').toUpperCase()
  return <span aria-hidden="true" className={`grid shrink-0 place-items-center overflow-hidden font-bold ${className} ${url ? '' : fallbackClassName}`}>
    {url ? <img src={url} alt="" className="h-full w-full object-cover" /> : letters}
  </span>
}

// The shell and greeting share these queries. Polling the small metadata
// payload also picks up an approval made in another signed-in browser.
export function OwnAvatar({ name, className, fallbackClassName }: { name: string; className?: string; fallbackClassName?: string }) {
  const { user, accessToken } = useAuth()
  const student = useQuery({
    queryKey: ['avatar', 'own-student', user?.id],
    enabled: !!accessToken && user?.role === 'student',
    queryFn: () => getMyStudent(accessToken!), retry: false, refetchInterval: 20000,
  })
  const staff = useQuery({
    queryKey: ['avatar', 'own-staff', user?.id],
    enabled: !!accessToken && ['teacher', 'head_teacher', 'admin', 'accountant'].includes(user?.role || ''),
    queryFn: () => getStaffProfile(accessToken!, 'me'), retry: false, refetchInterval: 20000,
  })
  const kind = user?.role === 'student' ? 'student' : 'staff'
  const id = kind === 'student' ? student.data?.id : staff.data?.id
  const version = kind === 'student' ? student.data?.photo_version : staff.data?.photo_version
  return <PersonAvatar name={name} kind={kind} id={id} version={version} className={className} fallbackClassName={fallbackClassName} />
}
