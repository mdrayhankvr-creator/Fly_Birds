// Taka amounts in words for receipts (QA-15), in English and Bangla, using
// the South Asian grouping: crore, lakh, thousand, hundred.

const enOnes = ['', 'One', 'Two', 'Three', 'Four', 'Five', 'Six', 'Seven', 'Eight', 'Nine', 'Ten', 'Eleven', 'Twelve', 'Thirteen', 'Fourteen', 'Fifteen', 'Sixteen', 'Seventeen', 'Eighteen', 'Nineteen']
const enTens = ['', '', 'Twenty', 'Thirty', 'Forty', 'Fifty', 'Sixty', 'Seventy', 'Eighty', 'Ninety']

// Bangla number names below 100 are irregular, so they are listed in full.
const bn = ('শূন্য এক দুই তিন চার পাঁচ ছয় সাত আট নয় দশ এগারো বারো তেরো চৌদ্দ পনেরো ষোলো সতেরো আঠারো উনিশ বিশ ' +
  'একুশ বাইশ তেইশ চব্বিশ পঁচিশ ছাব্বিশ সাতাশ আটাশ ঊনত্রিশ ত্রিশ একত্রিশ বত্রিশ তেত্রিশ চৌত্রিশ পঁয়ত্রিশ ছত্রিশ সাঁইত্রিশ আটত্রিশ ঊনচল্লিশ চল্লিশ ' +
  'একচল্লিশ বিয়াল্লিশ তেতাল্লিশ চুয়াল্লিশ পঁয়তাল্লিশ ছেচল্লিশ সাতচল্লিশ আটচল্লিশ ঊনপঞ্চাশ পঞ্চাশ একান্ন বাহান্ন তিপ্পান্ন চুয়ান্ন পঞ্চান্ন ছাপ্পান্ন সাতান্ন আটান্ন ঊনষাট ষাট ' +
  'একষট্টি বাষট্টি তেষট্টি চৌষট্টি পঁয়ষট্টি ছেষট্টি সাতষট্টি আটষট্টি ঊনসত্তর সত্তর একাত্তর বাহাত্তর তিয়াত্তর চুয়াত্তর পঁচাত্তর ছিয়াত্তর সাতাত্তর আটাত্তর ঊনআশি আশি ' +
  'একাশি বিরাশি তিরাশি চুরাশি পঁচাশি ছিয়াশি সাতাশি অষ্টাশি ঊননব্বই নব্বই একানব্বই বিরানব্বই তিরানব্বই চুরানব্বই পঁচানব্বই ছিয়ানব্বই সাতানব্বই আটানব্বই নিরানব্বই').split(' ')

function enBelowThousand(n: number): string[] {
  const words: string[] = []
  if (n >= 100) { words.push(enOnes[Math.floor(n / 100)], 'Hundred'); n %= 100 }
  if (n >= 20) { words.push(enTens[Math.floor(n / 10)] + (n % 10 ? '-' + enOnes[n % 10] : '')) } else if (n > 0) words.push(enOnes[n])
  return words
}

function bnBelowThousand(n: number): string[] {
  const words: string[] = []
  if (n >= 100) { words.push(bn[Math.floor(n / 100)] + 'শত'); n %= 100 }
  if (n > 0) words.push(bn[n])
  return words
}

// Splits a whole number into crore, lakh, thousand and the rest; the crore
// part may itself be large and is named recursively.
function group(n: number, below: (n: number) => string[], units: [string, string, string]): string[] {
  if (n === 0) return []
  const crore = Math.floor(n / 10_000_000), lakh = Math.floor((n % 10_000_000) / 100_000), thousand = Math.floor((n % 100_000) / 1000), rest = n % 1000
  return [
    ...(crore ? [...group(crore, below, units), units[0]] : []),
    ...(lakh ? [...below(lakh), units[1]] : []),
    ...(thousand ? [...below(thousand), units[2]] : []),
    ...below(rest),
  ]
}

function split(amount: number): [number, number] {
  const cents = Math.round(Math.abs(amount) * 100)
  return [Math.floor(cents / 100), cents % 100]
}

// "Taka One Thousand Five Hundred and Fifty Paisa Only"
export function amountInWordsEN(amount: number): string {
  const [taka, paisa] = split(amount)
  const takaWords = taka ? group(taka, enBelowThousand, ['Crore', 'Lakh', 'Thousand']).join(' ') : 'Zero'
  return `Taka ${takaWords}${paisa ? ` and ${enBelowThousand(paisa).join(' ')} Paisa` : ''} Only`
}

// "এক হাজার পাঁচশত টাকা ও পঞ্চাশ পয়সা মাত্র"
export function amountInWordsBN(amount: number): string {
  const [taka, paisa] = split(amount)
  const takaWords = taka ? group(taka, bnBelowThousand, ['কোটি', 'লক্ষ', 'হাজার']).join(' ') : bn[0]
  return `${takaWords} টাকা${paisa ? ` ও ${bn[paisa]} পয়সা` : ''} মাত্র`
}
