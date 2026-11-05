// Joins rendered parts (chunks of a large document) into one PDF, page by
// page, so a large class reports progress as each part is rendered.
#let doc = json(sys.inputs.at("data", default: "data.json"))
#set document(title: doc.title)
#set page(paper: doc.paper, flipped: doc.flipped, margin: 0pt)
#for part in doc.parts {
  for p in range(1, part.pages + 1) {
    image(part.file, page: p, width: 100%, height: 100%)
  }
}
