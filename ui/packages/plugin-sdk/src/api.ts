export interface APIRequestOptions extends RequestInit {
  retryOnUnauthorized?: boolean
}
export interface APIClient {
  request: (url: string, options?: APIRequestOptions) => Promise<Response>
  get: <T>(url: string, options?: APIRequestOptions) => Promise<T>
  post: <T>(
    url: string,
    data?: unknown,
    options?: APIRequestOptions
  ) => Promise<T>
  put: <T>(
    url: string,
    data?: unknown,
    options?: APIRequestOptions
  ) => Promise<T>
  patch: <T>(
    url: string,
    data?: unknown,
    options?: APIRequestOptions
  ) => Promise<T>
  delete: <T>(url: string, options?: APIRequestOptions) => Promise<T>
}
export let apiClient: APIClient
