use serde::{Deserialize, Serialize};

use crate::date::Date;
use crate::money::{Currency, ceil_div_nonnegative, mul_div_floor_nonnegative};
use crate::{EngineError, EngineResult};

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct GoalInput {
    pub currency: Currency,
    pub target_minor: i64,
    pub current_minor: i64,
    pub current_monthly_contribution_minor: i64,
    pub current_date: Date,
    pub target_date: Option<Date>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum GoalStatus {
    Completed,
    OnTrack,
    BehindPace,
    NoTargetDate,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct GoalOutput {
    pub currency: Currency,
    pub status: GoalStatus,
    pub remaining_minor: i64,
    pub progress_basis_points: i64,
    pub contribution_months_remaining: Option<u32>,
    pub required_monthly_contribution_minor: Option<i64>,
    pub expected_completion_date: Option<Date>,
}

pub fn calculate_goal(input: &GoalInput) -> EngineResult<GoalOutput> {
    input.current_date.validate()?;
    if input.target_minor <= 0
        || input.current_minor < 0
        || input.current_minor > input.target_minor
        || input.current_monthly_contribution_minor < 0
    {
        return Err(EngineError::InvalidInput(
            "goal inputs are outside the supported range",
        ));
    }
    if let Some(target_date) = input.target_date {
        target_date.validate()?;
        if target_date < input.current_date && input.current_minor < input.target_minor {
            return Err(EngineError::InvalidInput(
                "active goal target date is in the past",
            ));
        }
    }

    let remaining_minor = input
        .target_minor
        .checked_sub(input.current_minor)
        .ok_or(EngineError::ArithmeticOverflow)?;
    let progress_basis_points =
        mul_div_floor_nonnegative(input.current_minor, 10_000, input.target_minor)?;
    if remaining_minor == 0 {
        return Ok(GoalOutput {
            currency: input.currency,
            status: GoalStatus::Completed,
            remaining_minor,
            progress_basis_points,
            contribution_months_remaining: Some(0),
            required_monthly_contribution_minor: Some(0),
            expected_completion_date: Some(input.current_date),
        });
    }

    let expected_completion_date = if input.current_monthly_contribution_minor == 0 {
        None
    } else {
        let months =
            ceil_div_nonnegative(remaining_minor, input.current_monthly_contribution_minor)?;
        Some(
            input
                .current_date
                .add_months(u32::try_from(months).map_err(|_| EngineError::ArithmeticOverflow)?)?,
        )
    };
    let Some(target_date) = input.target_date else {
        return Ok(GoalOutput {
            currency: input.currency,
            status: GoalStatus::NoTargetDate,
            remaining_minor,
            progress_basis_points,
            contribution_months_remaining: None,
            required_monthly_contribution_minor: None,
            expected_completion_date,
        });
    };
    let months_remaining = input.current_date.calendar_months_until(target_date)?;
    let required_monthly_contribution_minor =
        ceil_div_nonnegative(remaining_minor, i64::from(months_remaining))?;
    let status = if input.current_monthly_contribution_minor >= required_monthly_contribution_minor
    {
        GoalStatus::OnTrack
    } else {
        GoalStatus::BehindPace
    };
    Ok(GoalOutput {
        currency: input.currency,
        status,
        remaining_minor,
        progress_basis_points,
        contribution_months_remaining: Some(months_remaining),
        required_monthly_contribution_minor: Some(required_monthly_contribution_minor),
        expected_completion_date,
    })
}
