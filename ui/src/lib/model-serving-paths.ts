// Paths of the model serving pages. Routes, links, the sidebar and the
// breadcrumbs all build from these, so moving a page is a change here. Routes
// pass `:param` placeholders to the builders to get their patterns.
const ROOT = '/model-serving'
const SERVICES = `${ROOT}/services`
const GATEWAY = `${ROOT}/gateway`
const MODELS = `${ROOT}/models`

type ID = number | string

export const MODEL_SERVING_PATHS = {
  root: ROOT,
  overview: `${ROOT}/overview`,
  services: SERVICES,
  serviceCreate: `${SERVICES}/create`,
  service: (namespace: string, name: string) =>
    `${SERVICES}/${namespace}/${name}`,
  serviceEdit: (namespace: string, name: string) =>
    `${SERVICES}/${namespace}/${name}/edit`,
  serviceScale: (namespace: string, name: string) =>
    `${SERVICES}/${namespace}/${name}/scale`,
  gateway: GATEWAY,
  callerCreate: `${GATEWAY}/callers/create`,
  callerEdit: (id: ID) => `${GATEWAY}/callers/${id}/edit`,
  callerQuota: (id: ID) => `${GATEWAY}/callers/${id}/quota`,
  usage: `${ROOT}/usage`,
  models: MODELS,
  modelCreate: `${MODELS}/create`,
  modelEdit: (id: ID) => `${MODELS}/${id}/edit`,
  storage: `${MODELS}/storage`,
  gpus: `${ROOT}/gpus`,
}
