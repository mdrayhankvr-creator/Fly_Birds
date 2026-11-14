// Section names may already contain a label entered by the school.
export function sectionDisplayName(value: string, language: string): string {
  const name = value.trim()
  if (!name) return ''
  if (/^section(?:\b|\s|[-:])/i.test(name) || /^শাখা(?:\s|[-:]|$)/u.test(name) || /(?:\s|[-:])শাখা$/u.test(name)) return name
  return `${language === 'bn' ? 'শাখা' : 'Section'} ${name}`
}
