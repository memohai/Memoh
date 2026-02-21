export type AuthFetcher = (
  url: string,
  options?: RequestInit,
) => Promise<Response>