import * as React from "react"
import { ChevronDownIcon } from "lucide-react"

import { cn } from "@/lib/utils"

function NativeSelect({
  className,
  children,
  ...props
}: React.ComponentProps<"select">) {
  return (
    <span className="relative block w-full" data-slot="native-select-wrapper">
      <select
        data-slot="native-select"
        className={cn(
          "min-h-11 w-full appearance-none rounded-none border border-input bg-background py-2 pr-10 pl-3 text-sm outline-none transition-colors focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:border-border disabled:bg-muted disabled:text-muted-foreground disabled:opacity-100 aria-invalid:border-foreground aria-invalid:ring-0",
          className
        )}
        {...props}
      >
        {children}
      </select>
      <ChevronDownIcon
        aria-hidden="true"
        className="pointer-events-none absolute top-1/2 right-3 size-4 -translate-y-1/2 text-muted-foreground"
      />
    </span>
  )
}

function NativeSelectOption(props: React.ComponentProps<"option">) {
  return <option data-slot="native-select-option" {...props} />
}

function NativeSelectOptGroup(props: React.ComponentProps<"optgroup">) {
  return <optgroup data-slot="native-select-optgroup" {...props} />
}

export { NativeSelect, NativeSelectOption, NativeSelectOptGroup }
