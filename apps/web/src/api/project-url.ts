// projectUrl builds a project-scoped URL; path is omitted when undefined.
export function projectUrl(pathname: string, project: string, path?: string): string {
  const params = new URLSearchParams({ project })
  if (path !== undefined) params.set("path", path)
  return `${pathname}?${params.toString()}`
}
