import { useState } from "react"

export function Counter({ start = 0 }: { start?: number }) {
  const [count, setCount] = useState<number>(start)
  return (
    <div className="counter" data-id={count}>
      <Button onClick={() => setCount(count + 1)}>Add {count}</Button>
      {/* comment */}
      &nbsp;<span>text</span>
    </div>
  )
}
