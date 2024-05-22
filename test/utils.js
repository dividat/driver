const { spawn } = require('child_process')
const WebSocket = require('ws')

module.exports = {
  wait: function (t) {
    return new Promise((resolve, reject) => {
      setTimeout(resolve, t)
    })
  },

  startDriver: function (...args) {
    return spawn('../bin/dividat-driver', args)
    // useful for debugging:
    // return spawn('../bin/dividat-driver', [], {stdio: 'inherit'})
  },

  connectWS: function (url, opts) {
    return new Promise((resolve, reject) => {
      const ws = new WebSocket(url, opts)
      ws.on('open', () => {
        ws.removeAllListeners()
        resolve(ws)
      }).on('error', reject)
    })
  },

  getJSON: function (uri) {
    return fetch(uri)
      .then(response => {
        if (!response.ok) {
          throw new Error(`Network response was not OK: ${response.status}`)
        }
        return response.json()
    })
  },

  expectEvent: function (emitter, event, filter) {
    return new Promise((resolve, reject) => {
      // TODO: remove listener once resolved
      emitter.on(event, (a) => {
        try {
          if (filter(a)) {
            resolve(a)
          }
        } catch (e) {
          //
        }
      })
    })
  }
}
