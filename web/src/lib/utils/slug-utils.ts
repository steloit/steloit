/**
 * Slug utilities for composite slugs with embedded IDs
 * Enables cross-organization access with user-friendly URLs
 */

const UUID_LENGTH = 36
const UUID_PATTERN = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/

/**
 * Generate a composite slug from name and ID
 * @param name - Human readable name (e.g., "Steloit Technologies")
 * @param id - UUID identifier (e.g., "018f6b6a-1234-7abc-8def-0123456789ab")
 * @returns Composite slug (e.g., "steloit-technologies-018f6b6a-1234-7abc-8def-0123456789ab")
 * @throws Error if name is empty or ID is invalid
 */
export function generateCompositeSlug(name: string, id: string): string {
  if (!name || !name.trim()) {
    throw new Error('Organization/Project name cannot be empty')
  }

  if (!id || !UUID_PATTERN.test(id)) {
    throw new Error(`Invalid UUID format: ${id}`)
  }

  // Convert name to slug format
  let nameSlug = name
    .toLowerCase()
    .trim()
    .replace(/[^\w\s-]/g, '') // Remove special characters
    .replace(/[\s_-]+/g, '-') // Replace spaces and underscores with hyphens
    .replace(/^-+|-+$/g, '') // Remove leading/trailing hyphens

  // Handle edge case: all characters removed (special chars only)
  if (!nameSlug) {
    nameSlug = 'org' // Fallback for names with only special characters
  }

  // Truncate to reasonable length to avoid URL length limits
  if (nameSlug.length > 50) {
    nameSlug = nameSlug.substring(0, 50).replace(/-+$/, '')
  }

  return `${nameSlug}-${id}`
}

/**
 * Extract the original ID from a composite slug
 * @param compositeSlug - Composite slug (e.g., "steloit-technologies-018f6b6a-1234-7abc-8def-0123456789ab")
 * @returns Original UUID (e.g., "018f6b6a-1234-7abc-8def-0123456789ab")
 */
export function extractIdFromCompositeSlug(compositeSlug: string): string {
  if (compositeSlug.length < UUID_LENGTH) {
    throw new Error(`Invalid composite slug format: ${compositeSlug}`)
  }

  const urlId = compositeSlug.slice(-UUID_LENGTH)

  if (!UUID_PATTERN.test(urlId)) {
    throw new Error(`Invalid composite slug format: ${compositeSlug}`)
  }

  return urlId
}

/**
 * Extract the name slug portion from a composite slug
 * @param compositeSlug - Composite slug (e.g., "steloit-technologies-018f6b6a-1234-7abc-8def-0123456789ab")
 * @returns Name slug portion (e.g., "steloit-technologies")
 */
export function extractNameSlugFromCompositeSlug(compositeSlug: string): string {
  // Remove the last 37 characters (36 for UUID + 1 for hyphen)
  return compositeSlug.slice(0, -(UUID_LENGTH + 1))
}

/**
 * Validate if a string looks like a composite slug
 * @param slug - String to validate
 * @returns True if it appears to be a valid composite slug
 */
export function isValidCompositeSlug(slug: string): boolean {
  if (slug.length < UUID_LENGTH + 2) return false // min: "x-" + UUID
  const uuidPart = slug.slice(-UUID_LENGTH)
  return slug[slug.length - UUID_LENGTH - 1] === '-' && UUID_PATTERN.test(uuidPart)
}

/**
 * Check if a slug is a legacy slug (no embedded ID)
 * @param slug - String to check
 * @returns True if it's a legacy slug format
 */
export function isLegacySlug(slug: string): boolean {
  return !isValidCompositeSlug(slug)
}

/**
 * Build URL for organization with composite slug
 * @param name - Organization name
 * @param id - Organization ID
 * @param path - Optional sub-path
 * @returns Organization URL (e.g., "/organizations/steloit-tech-018f6b6a-1234-7abc-8def-0123456789ab")
 */
export function buildOrgUrl(name: string, id: string, path: string = ''): string {
  const compositeSlug = generateCompositeSlug(name, id)
  const basePath = `/organizations/${compositeSlug}`

  if (!path || path === '/') return basePath

  // Ensure path starts with /
  const normalizedPath = path.startsWith('/') ? path : `/${path}`
  return `${basePath}${normalizedPath}`
}

/**
 * Build URL for project with composite slug
 * @param name - Project name
 * @param id - Project ID
 * @param path - Optional sub-path
 * @returns Project URL (e.g., "/projects/analytics-platform-018f6b6a-1234-7abc-8def-0123456789ab")
 */
export function buildProjectUrl(name: string, id: string, path: string = ''): string {
  const compositeSlug = generateCompositeSlug(name, id)
  const basePath = `/projects/${compositeSlug}`

  if (!path || path === '/') return basePath

  // Ensure path starts with /
  const normalizedPath = path.startsWith('/') ? path : `/${path}`
  return `${basePath}${normalizedPath}`
}

/**
 * Parse pathname to extract organization and project composite slugs
 * @param pathname - URL pathname
 * @returns Object with orgSlug and projectSlug (null if not found)
 */
export function parsePathContext(pathname: string): {
  orgSlug: string | null
  projectSlug: string | null
} {
  const segments = pathname.split('/').filter(Boolean)

  const orgIndex = segments.indexOf('organizations')
  const orgSlug = (orgIndex !== -1 && segments[orgIndex + 1])
    ? segments[orgIndex + 1]
    : null

  const projectIndex = segments.indexOf('projects')
  const projectSlug = (projectIndex !== -1 && segments[projectIndex + 1])
    ? segments[projectIndex + 1]
    : null

  return { orgSlug, projectSlug }
}

/**
 * Get or generate slug for an organization
 * @param org - Organization object
 * @returns Composite slug (uses org.slug if available, otherwise generates from name + id)
 */
export function getOrgSlug(org: { name: string; id: string; slug?: string }): string {
  return org.slug || generateCompositeSlug(org.name, org.id)
}

/**
 * Get or generate slug for a project
 * @param project - Project object
 * @returns Composite slug (uses project.slug if available, otherwise generates from name + id)
 */
export function getProjectSlug(project: { name: string; id: string; slug?: string }): string {
  return project.slug || generateCompositeSlug(project.name, project.id)
}
