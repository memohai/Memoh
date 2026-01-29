import axios, { type AxiosRequestConfig } from 'axios'
import { useRouter } from 'vue-router'

const router=useRouter()
export default (function () {
  const axiosInstance = axios.create({
    baseURL: '/api'
  })

  axiosInstance.interceptors.response.use((response) => {
    return response
  }, (error) => { 
    if (error?.status === 401) {
      router.replace({
        name:'Login'
      })
    }
    return Promise.reject(error)
  })
  return (params: AxiosRequestConfig,isToken=true) => {
    axiosInstance.interceptors.request.use((config) => {
      if (isToken) {
        const token = localStorage.getItem('token')
        config.headers['Authorization'] =`Bearer ${token}`
      }
      return config
    }, (error) => Promise.reject(error))
  
    return axiosInstance(params)
  }
}())