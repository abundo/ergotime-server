import axios from 'axios'
import router from '@/router'

/**
 * Shared Axios instance.
 * credentials: 'include' so httpOnly session cookie is sent (same-origin / CORS with credentials).
 * Responses are JSON; never inject user-controlled HTML (XSS defense).
 */
const api = axios.create({
  baseURL: '/api',
  timeout: 30000,
  withCredentials: true,
  headers: {
    'Content-Type': 'application/json',
    Accept: 'application/json',
    'X-Requested-With': 'XMLHttpRequest',
  },
})

api.interceptors.response.use(
  (res) => res,
  (err) => {
    if (err.response?.status === 401) {
      const path = router.currentRoute.value?.path
      if (path && path !== '/login') {
        router.push({ name: 'login', query: { redirect: path } })
      }
    }
    return Promise.reject(err)
  },
)

export default api

export function errorMessage(err, fallback = 'Request failed') {
  return err?.response?.data?.message || err?.message || fallback
}
