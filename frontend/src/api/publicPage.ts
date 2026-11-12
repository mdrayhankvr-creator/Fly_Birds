import { ApiError, apiBaseUrl, bearerRequest, json } from './client'

// The public school page below the sign-in form (checkpoint 5). A null
// section has no content and is not shown.
export type GalleryPhoto = { id: string; position: number; width: number; height: number; version: string; caption_en: string; caption_bn: string }
export type PublicNotice = { id: string; title_en: string; title_bn: string; body_en: string; body_bn: string; priority: 'normal' | 'important' | 'urgent'; publish_at: string }
export type PublicView = {
  design_style: PublicDesignStyle
  about: { about_en: string; about_bn: string; founded_year?: number; eiin?: string; students?: number; teachers?: number; pass_rate?: number } | null
  head: { name_en: string; name_bn: string; message_en: string; message_bn: string; photo_version?: string } | null
  gallery: GalleryPhoto[]
  notices: PublicNotice[]
  contact: { address_en: string; address_bn: string; email: string; map_url: string } | null
}
export type PublicDesignStyle = 'modern_bold' | 'classic' | 'minimal'
export type PublicPage = {
  design_style: PublicDesignStyle; pass_rate: number | null
  about_en: string; about_bn: string; founded_year: number | null; eiin: string; show_counts: boolean
  head_name_en: string; head_name_bn: string; head_message_en: string; head_message_bn: string; head_photo_version: string
  contact_address_en: string; contact_address_bn: string; contact_phone: string; contact_email: string; map_url: string
}
export type PublicPageInput = Omit<PublicPage, 'head_photo_version'>

export async function getPublicPage(signal?: AbortSignal): Promise<PublicView> {
  const response = await fetch(`${apiBaseUrl}/school/public-page`, { signal, credentials: 'same-origin' })
  if (!response.ok) throw new ApiError(response.status, 'request_failed')
  return (await response.json()).data as PublicView
}
export const headPhotoUrl = (version: string) => `${apiBaseUrl}/school/public-page/head-photo?v=${encodeURIComponent(version)}`
export const galleryImageUrl = (p: GalleryPhoto, thumb = false) => `${apiBaseUrl}/school/gallery/${encodeURIComponent(p.id)}/image?${thumb ? 'size=thumb&' : ''}v=${p.version}`

export const getPublicPageForEdit = (token: string) => bearerRequest<{ page: PublicPage; gallery: GalleryPhoto[] }>('/school/public-page/edit', token)
export const savePublicPage = (token: string, input: PublicPageInput) => bearerRequest<PublicPage>('/school/public-page', token, json('PUT', input))
export const saveHeadPhoto = (token: string, image: Blob) => bearerRequest<PublicPage>('/school/public-page/head-photo', token, { method: 'PUT', headers: { 'Content-Type': image.type }, body: image })
export const deleteHeadPhoto = (token: string) => bearerRequest<PublicPage>('/school/public-page/head-photo', token, { method: 'DELETE' })
export const addGalleryPhoto = (token: string, image: Blob) => bearerRequest<GalleryPhoto>('/school/gallery', token, { method: 'POST', headers: { 'Content-Type': image.type }, body: image })
export const updateGalleryPhoto = (token: string, id: string, caption_en: string, caption_bn: string) => bearerRequest<GalleryPhoto>(`/school/gallery/${encodeURIComponent(id)}`, token, json('PATCH', { caption_en, caption_bn }))
export const deleteGalleryPhoto = (token: string, id: string) => bearerRequest<{ deleted: boolean }>(`/school/gallery/${encodeURIComponent(id)}`, token, { method: 'DELETE' })
