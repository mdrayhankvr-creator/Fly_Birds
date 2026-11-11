// Marksheet v2 (ফলাফল পত্র / Academic Transcript): A4 portrait, one page per
// student, print-friendly (thin rules, light tints that survive grey
// printing).
#import "common.typ": *
#show: base
#set page(paper: "a4", margin: (x: 11mm, top: 9mm, bottom: 8mm), foreground: watermark)
#set text(size: 8.4pt)

#let L = doc.labels

#let header(st) = {
  grid(columns: (21mm, 1fr, 21mm), column-gutter: 3mm, align: (left + horizon, center + horizon, right + horizon),
    crest(19mm),
    school-lines(),
    if st.qr != "" {
      stack(spacing: 0.7mm, image(st.qr, width: 18mm), text(size: 5.3pt, fill: muted, L.scan))
    } else { [] },
  )
  v(1.8mm)
  provisional-banner()
  // Title band: both names of the document, then the exam and session.
  block(width: 100%, fill: tint, stroke: (left: 3pt + accent, rest: 0.5pt + accent.lighten(55%)), inset: (x: 3.5mm, y: 2mm), radius: 2pt,
    grid(columns: (1fr, auto), align: (left + horizon, right + horizon), column-gutter: 3mm,
      {
        text(size: 12pt, weight: "bold", tracking: 0.3pt, fill: accent.darken(25%), doc.heading)
        if doc.heading_alt != "" { text(size: 9.5pt, weight: "semibold", fill: muted, "  /  " + doc.heading_alt) }
      },
      { text(size: 9pt, weight: "bold", doc.exam_name); text(size: 8pt, fill: muted, "  ·  " + L.session + " " + doc.session_name) }))
}

#let student-block(st) = {
  let field(k, v) = (text(size: 7pt, fill: muted, k), text(weight: "semibold", v))
  grid(columns: (1fr, 23mm), column-gutter: 3.5mm, align: (left + top, right + top),
    block(width: 100%, stroke: 0.5pt + rule, radius: 2pt, inset: (x: 3mm, y: 2.2mm),
      grid(columns: (auto, 1.35fr, auto, 1fr), column-gutter: 2.4mm, row-gutter: 1.9mm, align: left + horizon,
        text(size: 7pt, fill: muted, L.name),
        { text(weight: "bold", size: 9.2pt, st.name); if st.name_alt != "" { text(size: 7.6pt, fill: muted, "  " + st.name_alt) } },
        ..field(L.student_id, st.student_id),
        ..field(L.father, st.father), ..field(L.class, st.class),
        ..field(L.mother, st.mother), ..field(L.section, st.section),
        ..field(L.dob, st.dob), ..field(L.roll, st.roll),
        ..field(L.session, st.session), ..field(L.exam, st.exam),
      )),
    box(width: 23mm, height: 27.5mm, stroke: 0.5pt + rule, radius: 2pt, clip: true, fill: tint,
      if st.photo != "" { image(st.photo, width: 23mm, height: 27.5mm, fit: "cover") }
      else { align(center + horizon, text(size: 7pt, fill: faint, L.photo)) }),
  )
}

// A mark with the full marks it is out of, small.
#let mark(c) = { c.obt; if c.full != "" { text(size: 6pt, fill: muted, "/" + c.full) } }

#let subject-table(st) = {
  let cols = doc.columns
  let ncols = 2 + cols.len() + (if doc.show_ct { 1 } else { 0 }) + 4
  let head = (L.code, L.subject, ..cols, ..if doc.show_ct { (L.ct,) } else { () }, L.total, L.highest, L.grade, L.gp)
  let widths = (auto, 1fr, ..cols.map(_ => 15mm), ..if doc.show_ct { (11mm,) } else { () }, 17mm, 12mm, 10mm, 10mm)
  // One row per paper; a combined subject's last cells span its papers.
  let emit(r) = {
    let papers = if r.papers.len() > 1 { r.papers } else { ((code: r.code, subject: r.subject, parts: r.parts),) }
    let n = papers.len()
    let out = ()
    for (i, p) in papers.enumerate() {
      out.push(text(fill: muted, p.code))
      out.push({
        p.subject
        if r.optional and i == 0 { h(1mm); pill(L.fourth) }
        if r.note != "" and i == n - 1 { linebreak(); text(size: 6.3pt, fill: muted, r.note) }
      })
      for c in p.parts { out.push(mark(c)) }
      if i == 0 {
        let span(x) = table.cell(rowspan: n, x)
        if doc.show_ct { out.push(span(r.ct)) }
        out.push(span({ text(weight: "bold", r.total); if r.full != "" { text(size: 6pt, fill: muted, "/" + r.full) } }))
        out.push(span(text(fill: muted, r.highest)))
        out.push(span(text(weight: "bold", fill: if r.failed { fail-red } else { ink }, r.grade)))
        out.push(span(r.gp))
      }
    }
    out
  }
  table(
    columns: widths,
    stroke: 0.4pt + rule,
    inset: (x: 1.3mm, y: 1.35mm),
    align: (x, y) => if x == 1 { left + horizon } else { center + horizon },
    fill: (x, y) => if y == 0 { tint } else { none },
    table.header(..head.map(h => text(size: 6.8pt, weight: "bold", fill: muted, h))),
    ..st.rows.map(emit).flatten(),
    ..if st.optional.len() > 0 {
      (table.cell(colspan: ncols, align: left + horizon, fill: tint.lighten(40%), text(size: 6.8pt, weight: "semibold", fill: accent.darken(25%), L.optional_heading)),)
    } else { () },
    ..st.optional.map(emit).flatten(),
  )
}

#let summary(st) = {
  let tile(k, v, strong: false, tone: ink) = block(width: 100%, inset: (x: 2.2mm, y: 1.7mm), radius: 2pt,
    stroke: if strong { 1pt + accent } else { 0.5pt + rule }, fill: if strong { tint } else { none },
    { text(size: 6.5pt, fill: muted, k); linebreak(); text(size: if strong { 11.5pt } else { 9.2pt }, weight: "bold", fill: tone, v) })
  grid(columns: (1fr,) * 5, column-gutter: 1.8mm, row-gutter: 1.8mm,
    tile(L.total_marks, st.total), tile(L.gpa, st.gpa, strong: true), tile(L.gpa_without, st.gpa_without), tile(L.letter, st.letter),
    tile(L.result, st.result, strong: true, tone: if st.failed { fail-red } else { ink }),
    tile(L.failed_count, st.failed_count, tone: if st.failed_count != "০" and st.failed_count != "0" and st.failed_count != "—" { fail-red } else { ink }),
    tile(L.class_pos, st.class_pos), tile(L.section_pos, st.section_pos), tile(L.attendance, st.attendance), tile(L.published, st.published),
  )
}

#let comment-box(st) = block(width: 100%, stroke: 0.5pt + rule, radius: 2pt, inset: (x: 3mm, y: 2mm), {
  text(size: 7pt, weight: "bold", fill: muted, L.comment)
  v(0.6mm)
  if st.comment != "" {
    text(size: 8.6pt, style: "italic", st.comment)
  } else {
    // Room to write by hand.
    v(4mm); line(length: 100%, stroke: 0.4pt + rule); v(3.5mm); line(length: 100%, stroke: 0.4pt + rule)
  }
})

#let grading() = if doc.grading.len() > 0 {
  text(size: 6.6pt, weight: "bold", fill: muted, L.grading)
  v(0.5mm)
  table(columns: (auto,) * 3, stroke: 0.4pt + rule, inset: (x: 1.4mm, y: 0.7mm), align: center + horizon,
    ..(L.marks_range, L.grade, L.point).map(h => text(size: 6pt, weight: "bold", fill: muted, h)),
    ..doc.grading.map(g => (text(size: 6pt, g.range), text(size: 6pt, weight: "semibold", g.grade), text(size: 6pt, g.point))).flatten())
}

#let signatures(st) = {
  let slot(sig) = align(center + bottom, {
    // The image is fitted inside the slot, whatever its proportions.
    box(width: 100%, height: 10mm, if sig.image != "" { align(center + bottom, image(sig.image, width: 100%, height: 100%, fit: "contain")) })
    line(length: 100%, stroke: 0.6pt + muted)
    v(-1mm)
    text(size: 7.4pt, weight: "semibold", sig.title)
    // Every slot keeps a second line, so the signature lines align.
    linebreak()
    text(size: 6.4pt, fill: muted, if sig.date { L.sig_date + ": ____ / ____ / ________" } else if sig.name != "" { sig.name } else { sym.zws })
  })
  grid(columns: (1fr,) * st.signatures.len(), column-gutter: 9mm, ..st.signatures.map(slot))
}

#let card(st) = {
  header(st)
  v(2.2mm)
  student-block(st)
  v(2.4mm)
  if st.withheld {
    block(width: 100%, inset: 6mm, radius: 3pt, stroke: 0.8pt + accent, fill: tint,
      align(center, { text(size: 12pt, weight: "bold", L.withheld); linebreak(); text(fill: muted, L.withheld_note) }))
  } else {
    subject-table(st)
    v(2.4mm)
    summary(st)
  }
  v(2.4mm)
  grid(columns: (1fr, auto), column-gutter: 4mm, align: (left + top, left + top), comment-box(st), grading())
  v(1fr)
  signatures(st)
  v(1.6mm)
  line(length: 100%, stroke: 0.4pt + rule)
  v(-0.7mm)
  text(size: 6.2pt, fill: muted, st.footer)
}

#for (i, st) in doc.students.enumerate() {
  if i > 0 { pagebreak() }
  card(st)
}
