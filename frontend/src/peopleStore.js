import { ref } from 'vue'
import { api } from './api.js'

export const people = ref([])

export async function loadPeople() {
  try {
    people.value = (await api.listPeople()) || []
  } catch (e) {
    console.error('listPeople:', e)
    people.value = []
  }
  return people.value
}
