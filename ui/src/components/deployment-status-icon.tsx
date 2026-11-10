import {
  IconCircleCheckFilled,
  IconCircleXFilled,
  IconExclamationCircle,
  IconLoader,
  IconPlayerPause,
  IconTrash,
  IconTrendingDown,
} from '@tabler/icons-react'

import { DeploymentStatusType } from '@/types/k8s'

interface DeploymentStatusIconProps {
  status: DeploymentStatusType
  className?: string
  showAnimation?: boolean
}

export const DeploymentStatusIcon = ({
  status,
  className = '',
  showAnimation = true,
}: DeploymentStatusIconProps) => {
  const animationClass = showAnimation ? 'animate-spin' : ''

  switch (status) {
    case 'Available':
      return <IconCircleCheckFilled className={`fill-success ${className}`} />

    case 'Unknown':
    case 'Not Available':
      return <IconCircleXFilled className={`fill-destructive ${className}`} />

    case 'Terminating':
      return <IconTrash className={`text-warning ${className}`} />

    case 'Paused':
      return <IconPlayerPause className={`fill-info ${className}`} />

    case 'Scaled Down':
      return (
        <IconTrendingDown className={`text-muted-foreground ${className}`} />
      )

    case 'Progressing':
      return (
        <IconLoader className={`${animationClass} text-info ${className}`} />
      )

    default:
      return (
        <IconExclamationCircle
          className={`fill-muted-foreground ${className}`}
        />
      )
  }
}
