// slipDate formats the date a temporary password stops working, as printed
// on a slip: day, month and year in the school's time zone.
export function slipDate(iso: string, language: string): string {
  const at = new Date(iso)
  if (Number.isNaN(at.getTime())) return ''
  return new Intl.DateTimeFormat(language === 'bn' ? 'bn-BD' : 'en-GB', { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'Asia/Dhaka' }).format(at)
}
