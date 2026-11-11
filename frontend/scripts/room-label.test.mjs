// Unit tests for the room label: "Room" is printed once, never twice.
// Run with: node --experimental-strip-types --test scripts/room-label.test.mjs
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { roomText } from '../src/components/roomLabel.ts'

const en = room => `Room ${room}`
const bn = room => `কক্ষ: ${room}`

test('names that already say room are printed alone', () => {
  assert.equal(roomText('Room 101', bn), 'Room 101')
  assert.equal(roomText('room 7', en), 'room 7')
  assert.equal(roomText('কক্ষ ১০১', bn), 'কক্ষ ১০১')
  assert.equal(roomText('Classroom 5', en), 'Classroom 5')
  assert.equal(roomText('শ্রেণিকক্ষ ৫', bn), 'শ্রেণিকক্ষ ৫')
  assert.equal(roomText('হলরুম', bn), 'হলরুম')
  assert.equal(roomText('  Room 3 ', en), 'Room 3')
})

test('other names get the label', () => {
  assert.equal(roomText('101', en), 'Room 101')
  assert.equal(roomText('১০১', bn), 'কক্ষ: ১০১')
  assert.equal(roomText('Hall', en), 'Room Hall')
  assert.equal(roomText('Science lab', bn), 'কক্ষ: Science lab')
  assert.equal(roomText('বিজ্ঞান গবেষণাগার', bn), 'কক্ষ: বিজ্ঞান গবেষণাগার')
})
