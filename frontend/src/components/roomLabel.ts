// A room's own name often already says "Room" ("Room 101", "কক্ষ ১০১",
// "Classroom 5", "হলরুম"). Adding the "Room" label to those would print it
// twice ("কক্ষ Room 101"), so the label is used only when the name does not
// say it.
const saysRoom = /room|কক্ষ|রুম/i

export function namesItself(room: string) {
  return saysRoom.test(room)
}

// roomText returns the room as it should be printed: the bare name when it
// already says "Room", otherwise the name inside the given label.
export function roomText(room: string, label: (room: string) => string) {
  const name = room.trim()
  return namesItself(name) ? name : label(name)
}
