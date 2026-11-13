// Marks a field a rule cannot be saved without, at the top right of its
// label. Screen readers rely on the input's required attribute instead.
export function RequiredMark() {
  return (
    <span aria-hidden className="-ml-1.5 self-start text-destructive">
      *
    </span>
  )
}
