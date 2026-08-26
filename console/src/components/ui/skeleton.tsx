import { cn } from "@/lib/utils"

function Skeleton({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="skeleton"
      className={cn(
        "rounded-none border border-dashed border-border bg-muted/60",
        className
      )}
      {...props}
    />
  )
}

export { Skeleton }
