import {
  IconAlertTriangle,
  IconCircleCheckFilled,
  IconCircleXFilled,
  IconExclamationCircle,
  IconPlayerPause,
} from '@tabler/icons-react'

export type NodeStatusType =
  | 'Ready'
  | 'NotReady'
  | 'Ready,SchedulingDisabled'
  | 'NotReady,SchedulingDisabled'
  | 'NetworkUnavailable'
  | 'MemoryPressure'
  | 'DiskPressure'
  | 'PIDPressure'
  | 'Unknown'

interface NodeStatusIconProps {
  status: NodeStatusType | string
  className?: string
}

export const NodeStatusIcon = ({
  status,
  className = '',
}: NodeStatusIconProps) => {
  switch (status) {
    case 'Ready':
      return <IconCircleCheckFilled className={`fill-success ${className}`} />

    case 'Ready,SchedulingDisabled':
      return <IconPlayerPause className={`fill-warning ${className}`} />

    case 'NotReady':
    case 'NotReady,SchedulingDisabled':
    case 'NetworkUnavailable':
      return <IconCircleXFilled className={`fill-destructive ${className}`} />

    case 'MemoryPressure':
    case 'DiskPressure':
    case 'PIDPressure':
      return <IconAlertTriangle className={`fill-warning ${className}`} />

    default:
      return (
        <IconExclamationCircle
          className={`fill-muted-foreground ${className}`}
        />
      )
  }
}
