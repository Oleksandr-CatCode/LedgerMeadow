use serde::{Deserialize, Serialize};

use crate::{EngineError, EngineResult};

pub const MIN_YEAR: i32 = 1970;
pub const MAX_YEAR: i32 = 9999;

#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Serialize, Deserialize)]
pub struct Date {
    pub year: i32,
    pub month: u8,
    pub day: u8,
}

impl Date {
    pub fn new(year: i32, month: u8, day: u8) -> EngineResult<Self> {
        let date = Self { year, month, day };
        date.validate()?;
        Ok(date)
    }

    pub fn validate(self) -> EngineResult<()> {
        if !(MIN_YEAR..=MAX_YEAR).contains(&self.year)
            || !(1..=12).contains(&self.month)
            || self.day == 0
            || self.day > days_in_month(self.year, self.month)
        {
            return Err(EngineError::InvalidInput(
                "date is outside the supported calendar",
            ));
        }
        Ok(())
    }

    pub fn ordinal(self) -> EngineResult<i32> {
        self.validate()?;
        let mut days = days_before_year(self.year) - days_before_year(MIN_YEAR);
        for month in 1..self.month {
            days += i32::from(days_in_month(self.year, month));
        }
        days.checked_add(i32::from(self.day) - 1)
            .ok_or(EngineError::ArithmeticOverflow)
    }

    pub fn days_until(self, other: Self) -> EngineResult<i32> {
        other
            .ordinal()?
            .checked_sub(self.ordinal()?)
            .ok_or(EngineError::ArithmeticOverflow)
    }

    pub fn add_days(self, days: u32) -> EngineResult<Self> {
        let target = self
            .ordinal()?
            .checked_add(i32::try_from(days).map_err(|_| EngineError::ArithmeticOverflow)?)
            .ok_or(EngineError::ArithmeticOverflow)?;
        from_ordinal(target)
    }

    pub fn previous_day(self) -> EngineResult<Self> {
        let target = self
            .ordinal()?
            .checked_sub(1)
            .ok_or(EngineError::ArithmeticOverflow)?;
        from_ordinal(target)
    }

    pub fn add_months(self, months: u32) -> EngineResult<Self> {
        self.validate()?;
        let current = i64::from(self.year)
            .checked_mul(12)
            .and_then(|value| value.checked_add(i64::from(self.month) - 1))
            .ok_or(EngineError::ArithmeticOverflow)?;
        let target = current
            .checked_add(i64::from(months))
            .ok_or(EngineError::ArithmeticOverflow)?;
        let year = i32::try_from(target / 12).map_err(|_| EngineError::ArithmeticOverflow)?;
        let month = u8::try_from((target % 12) + 1).map_err(|_| EngineError::ArithmeticOverflow)?;
        if !(MIN_YEAR..=MAX_YEAR).contains(&year) {
            return Err(EngineError::InvalidInput(
                "date recurrence exceeds the supported calendar",
            ));
        }
        Self::new(year, month, self.day.min(days_in_month(year, month)))
    }

    pub fn calendar_months_until(self, other: Self) -> EngineResult<u32> {
        self.validate()?;
        other.validate()?;
        if other < self {
            return Err(EngineError::InvalidInput(
                "target date precedes the current date",
            ));
        }
        let start = i64::from(self.year) * 12 + i64::from(self.month) - 1;
        let end = i64::from(other.year) * 12 + i64::from(other.month) - 1;
        let mut months = end - start;
        if other.day >= self.day {
            months += 1;
        }
        u32::try_from(months.max(1)).map_err(|_| EngineError::ArithmeticOverflow)
    }
}

pub fn days_in_month(year: i32, month: u8) -> u8 {
    match month {
        1 | 3 | 5 | 7 | 8 | 10 | 12 => 31,
        4 | 6 | 9 | 11 => 30,
        2 if is_leap_year(year) => 29,
        2 => 28,
        _ => 0,
    }
}

fn is_leap_year(year: i32) -> bool {
    year % 4 == 0 && (year % 100 != 0 || year % 400 == 0)
}

fn days_before_year(year: i32) -> i32 {
    let previous = year - 1;
    previous * 365 + previous / 4 - previous / 100 + previous / 400
}

fn from_ordinal(ordinal: i32) -> EngineResult<Date> {
    if ordinal < 0 {
        return Err(EngineError::InvalidInput(
            "date precedes the supported calendar",
        ));
    }
    let maximum = Date {
        year: MAX_YEAR,
        month: 12,
        day: 31,
    }
    .ordinal()?;
    if ordinal > maximum {
        return Err(EngineError::InvalidInput(
            "date exceeds the supported calendar",
        ));
    }

    let absolute = ordinal
        .checked_add(days_before_year(MIN_YEAR))
        .ok_or(EngineError::ArithmeticOverflow)?;
    let mut low = MIN_YEAR;
    let mut high = MAX_YEAR;
    while low < high {
        let middle = low + (high - low + 1) / 2;
        if days_before_year(middle) <= absolute {
            low = middle;
        } else {
            high = middle - 1;
        }
    }

    let year = low;
    let mut day_of_year = absolute - days_before_year(year);
    let mut month = 1_u8;
    while day_of_year >= i32::from(days_in_month(year, month)) {
        day_of_year -= i32::from(days_in_month(year, month));
        month += 1;
    }
    Date::new(
        year,
        month,
        u8::try_from(day_of_year + 1).map_err(|_| EngineError::ArithmeticOverflow)?,
    )
}
