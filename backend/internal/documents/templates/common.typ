// Shared look of every result document. All text arrives formatted (Bangla
// digits included) in data.json; these files only lay it out.
#let doc = json(sys.inputs.at("data", default: "data.json"))
#let accent = rgb(doc.school.accent)
#let ink = rgb("#0f172a")
#let muted = rgb("#475569")
#let faint = rgb("#94a3b8")
#let rule = rgb("#cbd5e1")
#let tint = accent.lighten(93%)
#let fail-red = rgb("#b91c1c")

#let base(body) = {
  set document(title: doc.title, author: doc.school.name)
  set text(font: ("Noto Sans", "Noto Sans Bengali"), size: 9pt, fill: ink, lang: doc.lang)
  set par(leading: 0.55em, spacing: 0.8em)
  body
}

// The school's mark: its logo, or its initials in a ring.
#let crest(size) = if doc.school.logo != "" {
  box(width: size, height: size, align(center + horizon, image(doc.school.logo, width: size, height: size, fit: "contain")))
} else {
  box(width: size, height: size, radius: 50%, stroke: 1.2pt + accent,
    align(center + horizon, text(size: size * 0.36, weight: "bold", fill: accent, doc.school.initials)))
}

// School name block: main language large, the other below, then contacts.
#let school-lines(centered: true) = {
  set align(if centered { center } else { left })
  text(size: 15pt, weight: "bold", doc.school.name)
  if doc.school.name_alt != "" { linebreak(); text(size: 10pt, weight: "semibold", fill: muted, doc.school.name_alt) }
  if doc.school.contact != "" { linebreak(); text(size: 7.5pt, fill: muted, doc.school.contact) }
}

#let provisional-banner() = if doc.provisional {
  block(width: 100%, fill: fail-red.lighten(90%), stroke: 0.8pt + fail-red, inset: (x: 3mm, y: 1.6mm), radius: 2pt,
    align(center, text(size: 8.5pt, weight: "bold", fill: fail-red, tracking: 0.6pt, doc.labels.provisional)))
}

#let watermark() = if doc.provisional {
  place(center + horizon, rotate(-32deg, text(size: 46pt, weight: "bold", fill: rgb(185, 28, 28, 38), doc.labels.provisional)))
}

#let pill(body, color: accent) = box(inset: (x: 1.2mm, y: 0.5mm), radius: 2pt, stroke: 0.5pt + color,
  text(size: 6.5pt, weight: "semibold", fill: color, body))
