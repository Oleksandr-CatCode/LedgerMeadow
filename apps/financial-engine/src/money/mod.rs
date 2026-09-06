use serde::{Deserialize, Serialize};

use crate::{EngineError, EngineResult};

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum Currency {
    Cad,
    Usd,
}

pub fn checked_add(left: i64, right: i64) -> EngineResult<i64> {
    left.checked_add(right)
        .ok_or(EngineError::ArithmeticOverflow)
}

pub fn checked_sub(left: i64, right: i64) -> EngineResult<i64> {
    left.checked_sub(right)
        .ok_or(EngineError::ArithmeticOverflow)
}

pub fn checked_sum(values: impl IntoIterator<Item = i64>) -> EngineResult<i64> {
    values.into_iter().try_fold(0_i64, checked_add)
}

pub fn mul_div_floor_nonnegative(value: i64, multiplier: i64, divisor: i64) -> EngineResult<i64> {
    validate_nonnegative_ratio(value, multiplier, divisor)?;
    let result = i128::from(value)
        .checked_mul(i128::from(multiplier))
        .ok_or(EngineError::ArithmeticOverflow)?
        / i128::from(divisor);
    i64::try_from(result).map_err(|_| EngineError::ArithmeticOverflow)
}

pub fn mul_div_ceil_nonnegative(value: i64, multiplier: i64, divisor: i64) -> EngineResult<i64> {
    validate_nonnegative_ratio(value, multiplier, divisor)?;
    let numerator = i128::from(value)
        .checked_mul(i128::from(multiplier))
        .ok_or(EngineError::ArithmeticOverflow)?;
    let result = numerator
        .checked_add(i128::from(divisor) - 1)
        .ok_or(EngineError::ArithmeticOverflow)?
        / i128::from(divisor);
    i64::try_from(result).map_err(|_| EngineError::ArithmeticOverflow)
}

pub fn mul_div_round_half_even_nonnegative(
    value: i64,
    multiplier: i64,
    divisor: i64,
) -> EngineResult<i64> {
    validate_nonnegative_ratio(value, multiplier, divisor)?;
    let numerator = i128::from(value)
        .checked_mul(i128::from(multiplier))
        .ok_or(EngineError::ArithmeticOverflow)?;
    let divisor = i128::from(divisor);
    let quotient = numerator / divisor;
    let remainder = numerator % divisor;
    let doubled = remainder
        .checked_mul(2)
        .ok_or(EngineError::ArithmeticOverflow)?;
    let rounded = if doubled > divisor || (doubled == divisor && quotient % 2 != 0) {
        quotient
            .checked_add(1)
            .ok_or(EngineError::ArithmeticOverflow)?
    } else {
        quotient
    };
    i64::try_from(rounded).map_err(|_| EngineError::ArithmeticOverflow)
}

pub fn ceil_div_nonnegative(value: i64, divisor: i64) -> EngineResult<i64> {
    mul_div_ceil_nonnegative(value, 1, divisor)
}

fn validate_nonnegative_ratio(value: i64, multiplier: i64, divisor: i64) -> EngineResult<()> {
    if value < 0 || multiplier < 0 || divisor <= 0 {
        return Err(EngineError::InvalidInput(
            "ratio operands must be nonnegative and the divisor must be positive",
        ));
    }
    Ok(())
}
