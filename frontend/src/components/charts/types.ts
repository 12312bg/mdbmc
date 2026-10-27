import type { Component } from 'vue'

export interface Segment {
  key: string
  label: string
  value: number
  color: string
  icon?: Component
}
