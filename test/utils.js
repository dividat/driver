const { spawn } = require('child_process')
const WebSocket = require('ws')

module.exports = {
  wait: function (t) {
    return new Promise((resolve, reject) => {
      setTimeout(resolve, t)
    })
  },

  startDriver: function (...args) {
    return spawn("bin/dividat-driver", args, {
      // uncomment for Driver logs when debugging:
      // stdio: "inherit",
    })
  },

  connectWS: function (url, opts, protocols) {
    return new Promise((resolve, reject) => {
      const ws = new WebSocket(url, protocols, opts)
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
  },

  // Checks whether a connection can be established via HTTP to URL, regardless
  // of status code.
  waitForEndpoint: async function(url, maxAttempts = 10, delay = 100) {
    let lastError = null
    for (let i = 0; i < maxAttempts; i++) {
      try {
        const res = await fetch(url)
        return
      } catch (e) {
        lastError = e
        await module.exports.wait(delay)
      }
    }
    throw new Error(`Endpoint ${url} not available after ${maxAttempts} attempts, last error: ${e}`)
  }
}
