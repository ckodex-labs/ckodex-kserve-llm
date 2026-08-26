import { cn } from "@/lib/utils"

function Spinner({ className, ...props }: React.ComponentProps<"span">) {
  return (
    <span
      role="status"
      aria-label="Loading"
      className={cn(
        "inline-flex size-4 items-center justify-center font-mono text-xs text-foreground",
        className
      )}
      {...props}
    >
      ○
    </span>
  )
}

export { Spinner }
