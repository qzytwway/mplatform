import type { TFunction } from 'i18next'

import { ProvisionNodeSpec, ProvisionSpec, ProvisionSSH } from '@/types/api'

import { isIPv4 } from './utils'

// A node row in the create form; key only identifies the row while editing.
export interface FormNode extends ProvisionNodeSpec {
  key: number
}

export interface ProvisionForm extends Omit<ProvisionSpec, 'nodes'> {
  nodes: FormNode[]
}

let lastNodeKey = 0
export const newNodeKey = () => ++lastNodeKey

export const DEFAULT_SSH: ProvisionSSH = {
  user: 'root',
  port: 22,
  authType: 'key',
  privateKey: '',
  password: '',
}

export const DEFAULT_ENDPOINT_PORT = 6443

const NAME_RE = /^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$/
const MAX_PORT = 65535

export function createDefaultForm(): ProvisionForm {
  return {
    name: '',
    description: '',
    kubernetesMinor: '',
    controlPlane: {
      mode: 'single',
      endpoint: '',
      endpointPort: DEFAULT_ENDPOINT_PORT,
    },
    network: {
      podCIDR: '10.244.0.0/16',
      serviceCIDR: '10.96.0.0/12',
      dnsDomain: 'cluster.local',
    },
    sources: { binaryMirror: '', imageRepository: '', registryMirrors: [] },
    ssh: DEFAULT_SSH,
    nodes: [],
    schedulableControlPlane: false,
    isDefault: false,
  }
}

// Sends only the secret of the chosen auth type.
const cleanSSH = (ssh: ProvisionSSH): ProvisionSSH => ({
  user: ssh.user.trim(),
  port: ssh.port,
  authType: ssh.authType,
  privateKey: ssh.authType === 'key' ? ssh.privateKey : '',
  password: ssh.authType === 'password' ? ssh.password : '',
})

export function toSpec(form: ProvisionForm): ProvisionSpec {
  const isSingle = form.controlPlane.mode === 'single'
  return {
    ...form,
    name: form.name.trim(),
    description: form.description.trim(),
    controlPlane: {
      mode: form.controlPlane.mode,
      endpoint: isSingle ? '' : form.controlPlane.endpoint.trim(),
      endpointPort: isSingle
        ? DEFAULT_ENDPOINT_PORT
        : form.controlPlane.endpointPort,
    },
    network: {
      podCIDR: form.network.podCIDR.trim(),
      serviceCIDR: form.network.serviceCIDR.trim(),
      dnsDomain: form.network.dnsDomain.trim(),
    },
    sources: {
      binaryMirror: form.sources.binaryMirror.trim(),
      imageRepository: form.sources.imageRepository.trim(),
      registryMirrors: form.sources.registryMirrors
        .map((mirror) => ({
          registry: mirror.registry.trim(),
          endpoint: mirror.endpoint.trim(),
        }))
        .filter((mirror) => mirror.registry || mirror.endpoint),
    },
    ssh: cleanSSH(form.ssh),
    nodes: form.nodes.map((node) => ({
      ip: node.ip.trim(),
      hostname: node.hostname.trim(),
      role: node.role,
      ssh: node.ssh ? cleanSSH(node.ssh) : null,
    })),
  }
}

const isValidPort = (port: number) =>
  Number.isInteger(port) && port > 0 && port <= MAX_PORT

export function validateSSH(ssh: ProvisionSSH, t: TFunction): string[] {
  const errors: string[] = []
  if (!ssh.user.trim()) errors.push(t('clusterProvision.validation.sshUser'))
  if (!isValidPort(ssh.port)) errors.push(t('clusterProvision.validation.port'))
  if (ssh.authType === 'key' && !ssh.privateKey.trim()) {
    errors.push(t('clusterProvision.validation.sshKey'))
  }
  if (ssh.authType === 'password' && !ssh.password) {
    errors.push(t('clusterProvision.validation.sshPassword'))
  }
  return errors
}

function validateNodes(form: ProvisionForm, t: TFunction): string[] {
  const errors: string[] = []
  if (form.nodes.length === 0) {
    return [t('clusterProvision.validation.noNodes')]
  }
  const seen = new Set<string>()
  for (const node of form.nodes) {
    const ip = node.ip.trim()
    if (!ip) errors.push(t('clusterProvision.validation.ipRequired'))
    else if (!isIPv4(ip))
      errors.push(t('clusterProvision.validation.invalidIP', { ip }))
    else if (seen.has(ip))
      errors.push(t('clusterProvision.validation.duplicateIP', { ip }))
    seen.add(ip)
    if (node.ssh) {
      for (const error of validateSSH(node.ssh, t)) {
        errors.push(t('clusterProvision.validation.nodeSSH', { ip, error }))
      }
    }
  }
  const controlPlanes = form.nodes.filter(
    (node) => node.role === 'control-plane'
  ).length
  if (form.controlPlane.mode === 'single' && controlPlanes !== 1) {
    errors.push(t('clusterProvision.validation.singleControlPlane'))
  }
  if (form.controlPlane.mode !== 'single' && controlPlanes !== 3) {
    errors.push(t('clusterProvision.validation.haControlPlanes'))
  }
  if (controlPlanes === form.nodes.length && !form.schedulableControlPlane) {
    errors.push(t('clusterProvision.validation.noWorkers'))
  }
  if (form.nodes.some((node) => !node.ssh)) {
    errors.push(...validateSSH(form.ssh, t))
  }
  return errors
}

export function validateForm(form: ProvisionForm, t: TFunction): string[] {
  const errors: string[] = []
  if (!NAME_RE.test(form.name.trim())) {
    errors.push(t('clusterProvision.validation.name'))
  }
  if (!form.kubernetesMinor) {
    errors.push(t('clusterProvision.validation.version'))
  }
  const { mode, endpoint, endpointPort } = form.controlPlane
  if (mode !== 'single') {
    if (!endpoint.trim()) {
      errors.push(t('clusterProvision.validation.endpoint'))
    } else if (mode === 'kube-vip' && !isIPv4(endpoint.trim())) {
      errors.push(t('clusterProvision.validation.vip'))
    }
    if (!isValidPort(endpointPort)) {
      errors.push(t('clusterProvision.validation.port'))
    }
  }
  errors.push(...validateNodes(form, t))
  if (
    form.sources.registryMirrors.some(
      (mirror) => !mirror.registry.trim() !== !mirror.endpoint.trim()
    )
  ) {
    errors.push(t('clusterProvision.validation.mirror'))
  }
  return errors
}
