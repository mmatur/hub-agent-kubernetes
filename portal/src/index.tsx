import React from 'react'
import { createRoot } from 'react-dom/client'

const { catalogName, catalogDescription } = window as any

function App() {
  fetch(`/api/${catalogName}/services`)
    .then((resp) => resp.json())
    .then((services) => console.log('services', services))
    .catch((err) => console.error(err))

  return (
    <div>
      <h3>{catalogName}</h3>
      <p>{catalogDescription}</p>
    </div>
  )
}

const container = document.getElementById('root')
if (!container) {
  throw new Error('Container not found')
}
const root = createRoot(container)

root.render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
