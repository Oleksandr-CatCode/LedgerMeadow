import { describe, expect, it } from 'vitest'
import {
  formatMoney,
  formatMoneyWithSign,
  minorToMoneyInput,
  parseMoneyInput,
} from './formatMoney'

describe('parseMoneyInput', () => {
  it.each([
    ['0', '0'],
    ['0.05', '5'],
    ['1', '100'],
    ['1.2', '120'],
    ['1.23', '123'],
    ['12.', '1200'],
    ['$1,234.50', '123450'],
    [' 0007.05 ', '705'],
  ])('converts %s exactly to minor units', (input, expected) => {
    expect(parseMoneyInput(input)).toBe(expected)
  })

  it.each(['', '$', '.50', '-1.00', '1.234', '1e3', 'NaN', '10 CAD'])(
    'rejects invalid input %s',
    (input) => {
      expect(parseMoneyInput(input)).toBeNull()
    },
  )
})

describe('formatMoney', () => {
  it('formats signed minor units without floating-point conversion', () => {
    expect(formatMoney('-123450', 'CAD')).toBe('-$1,234.50')
  })

  it('reports non-integer API values honestly', () => {
    expect(formatMoney('12.34', 'USD')).toBe('Invalid amount')
  })
})

describe('formatMoneyWithSign', () => {
  it('uses explicit transaction signs without floating-point conversion', () => {
    expect(formatMoneyWithSign('-14281', 'CAD')).toBe('−$142.81')
    expect(formatMoneyWithSign('318000', 'CAD')).toBe('+$3,180.00')
  })
})

describe('minorToMoneyInput', () => {
  it.each([
    ['0', '0.00'],
    ['5', '0.05'],
    ['120', '1.20'],
    ['123450', '1234.50'],
    ['-705', '-7.05'],
  ])('converts %s to an editable decimal string', (input, expected) => {
    expect(minorToMoneyInput(input)).toBe(expected)
  })

  it('rejects invalid API money', () => {
    expect(minorToMoneyInput('1.00')).toBeNull()
  })
})
