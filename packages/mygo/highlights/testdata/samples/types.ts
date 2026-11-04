/** Doc comment with @param tag. */
interface User<T extends object = {}> {
  readonly id: number
  name?: string
  meta: Record<string, T>
}
type Role = "admin" | "user"
enum Color { Red = 1, Green }
export function greet<T>(user: User<T>, role: Role = "user"): string {
  const label = user.name as string
  return `${label}:${role}` satisfies string
}
declare module "x" { export const y: unique symbol }
