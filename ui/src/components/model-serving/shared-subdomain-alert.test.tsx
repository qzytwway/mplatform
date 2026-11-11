/// <reference types="@testing-library/jest-dom" />
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { SharedSubdomainAlert } from './shared-subdomain-alert'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

describe('SharedSubdomainAlert', () => {
  it('gives the patch command for the service', () => {
    render(<SharedSubdomainAlert name="qwen" namespace="ai" />)

    const alert = screen.getByRole('alert')
    expect(alert).toHaveTextContent(
      'modelServing.services.sharedSubdomainTitle'
    )
    expect(alert).toHaveTextContent(
      'modelServing.services.sharedSubdomainDescription'
    )
    expect(
      screen.getByText(
        `kubectl patch lws qwen -n ai --type merge -p '{"spec":{"networkConfig":{"subdomainPolicy":"UniquePerReplica"}}}'`
      )
    ).toBeInTheDocument()
  })
})
