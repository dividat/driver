/* eslint-env mocha */
describe('General functionality', () => {
  require('./general')
})

describe('CORS and PNA protection', () => {
  require('./cors')
})

describe('Senso', () => {
  require('./senso')
})

describe("RFID", () => {
  require("./rfid")
})

describe("Flex", () => {
  require("./flex")
})
