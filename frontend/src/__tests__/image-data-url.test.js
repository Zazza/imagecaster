import { describe, it, expect } from 'vitest'
import { b64Url, imageMime } from '../imageDataUrl.js'

describe('b64Url', () => {
  it.each([
    ['/9j/4AAQSkZJRg', 'data:image/jpeg;base64,/9j/4AAQSkZJRg'],
    ['iVBORw0KGgoAAAANSUhEUg', 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUg'],
    ['UklGRiQAAABXRUJQ', 'data:image/webp;base64,UklGRiQAAABXRUJQ'],
    ['R0lGODlhAQAB', 'data:image/gif;base64,R0lGODlhAQAB'],
    ['AAAA', 'data:image/png;base64,AAAA'],
  ])('detects format of %s', (b64, want) => {
    expect(b64Url(b64)).toBe(want)
  })

  it('keeps data URLs and app image URLs as is', () => {
    expect(b64Url('data:image/jpeg;base64,/9j/')).toBe('data:image/jpeg;base64,/9j/')
    expect(b64Url('/api/img/12.jpg')).toBe('/api/img/12.jpg')
  })

  it('returns empty string for empty input', () => {
    expect(b64Url('')).toBe('')
    expect(b64Url(undefined)).toBe('')
    expect(b64Url(null)).toBe('')
  })

  it('falls back to png mime', () => {
    expect(imageMime('xyz')).toBe('image/png')
  })
})
