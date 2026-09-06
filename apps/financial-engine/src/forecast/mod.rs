use std::collections::{BTreeSet, HashSet};

use serde::{Deserialize, Serialize};

use crate::date::Date;
use crate::money::{Currency, checked_add};
use crate::{EngineError, EngineResult};

pub const MAX_PROJECTION_SOURCES: usize = 512;
pub const MAX_PROJECTION_EVENTS: usize = 4_096;
pub const MAX_PROJECTION_DAYS: i32 = 3_660;
const MAX_IDENTIFIER_BYTES: usize = 128;
const MAX_NAME_BYTES: usize = 200;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum ProjectionKind {
    Salary,
    Bill,
    Subscription,
    LoanPayment,
    PlannedTransfer,
    GoalContribution,
    Other,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum Frequency {
    Once,
    Weekly,
    Biweekly,
    Monthly,
    Quarterly,
    Annually,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct ProjectionSource {
    pub source_id: String,
    pub name: String,
    pub kind: ProjectionKind,
    pub amount_minor: i64,
    pub first_date: Date,
    pub frequency: Frequency,
    pub end_date: Option<Date>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct ProjectionInput {
    pub currency: Currency,
    pub starting_balance_minor: i64,
    pub start_date: Date,
    pub end_date: Date,
    pub safety_floor_minor: i64,
    pub max_events: u32,
    pub sources: Vec<ProjectionSource>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum ProjectionStatus {
    OnTrack,
    Watch,
    AtRisk,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct TimelineEvent {
    pub source_id: String,
    pub occurrence: u32,
    pub date: Date,
    pub kind: ProjectionKind,
    pub name: String,
    pub amount_minor: i64,
    pub balance_before_minor: i64,
    pub balance_after_minor: i64,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct ProjectionOutput {
    pub currency: Currency,
    pub ending_balance_minor: i64,
    pub minimum_balance_minor: i64,
    pub minimum_balance_date: Date,
    pub status: ProjectionStatus,
    pub events: Vec<TimelineEvent>,
}

#[derive(Debug, Clone)]
struct ExpandedEvent {
    source_id: String,
    occurrence: u32,
    date: Date,
    kind: ProjectionKind,
    name: String,
    amount_minor: i64,
}

pub fn build_projection(input: &ProjectionInput) -> EngineResult<ProjectionOutput> {
    validate_input(input)?;
    let event_limit =
        usize::try_from(input.max_events).map_err(|_| EngineError::ArithmeticOverflow)?;
    let mut expanded = Vec::with_capacity(event_limit.min(input.sources.len()));
    for source in &input.sources {
        expand_source(
            source,
            input.start_date,
            input.end_date,
            event_limit,
            &mut expanded,
        )?;
    }
    expanded.sort_by(|left, right| {
        left.date
            .cmp(&right.date)
            .then_with(|| left.source_id.cmp(&right.source_id))
            .then_with(|| left.occurrence.cmp(&right.occurrence))
    });

    let mut balance = input.starting_balance_minor;
    let mut minimum_balance = balance;
    let mut minimum_balance_date = input.start_date;
    let mut events = Vec::with_capacity(expanded.len());
    for event in expanded {
        let balance_before_minor = balance;
        balance = checked_add(balance, event.amount_minor)?;
        if balance < minimum_balance {
            minimum_balance = balance;
            minimum_balance_date = event.date;
        }
        events.push(TimelineEvent {
            source_id: event.source_id,
            occurrence: event.occurrence,
            date: event.date,
            kind: event.kind,
            name: event.name,
            amount_minor: event.amount_minor,
            balance_before_minor,
            balance_after_minor: balance,
        });
    }
    let status = if minimum_balance < 0 {
        ProjectionStatus::AtRisk
    } else if minimum_balance < input.safety_floor_minor {
        ProjectionStatus::Watch
    } else {
        ProjectionStatus::OnTrack
    };
    Ok(ProjectionOutput {
        currency: input.currency,
        ending_balance_minor: balance,
        minimum_balance_minor: minimum_balance,
        minimum_balance_date,
        status,
        events,
    })
}

pub fn pay_cycle_horizon(start_date: Date, sources: &[ProjectionSource]) -> EngineResult<Date> {
    start_date.validate()?;
    validate_sources(sources)?;

    let mut paycheck_dates = BTreeSet::new();
    for source in sources
        .iter()
        .filter(|source| source.kind == ProjectionKind::Salary)
    {
        let occurrence = first_relevant_occurrence(source, start_date)?;
        let first_date = occurrence_date(source, occurrence)?;
        if first_date >= start_date
            && source
                .end_date
                .is_none_or(|end_date| first_date <= end_date)
        {
            paycheck_dates.insert(first_date);
        }
        if first_date == start_date && source.frequency != Frequency::Once {
            let next_occurrence = occurrence
                .checked_add(1)
                .ok_or(EngineError::ArithmeticOverflow)?;
            let next_date = occurrence_date(source, next_occurrence)?;
            if source.end_date.is_none_or(|end_date| next_date <= end_date) {
                paycheck_dates.insert(next_date);
            }
        }
    }

    match paycheck_dates.into_iter().find(|date| *date > start_date) {
        Some(next_paycheck) => next_paycheck.previous_day(),
        None => start_date.add_months(1),
    }
}

fn validate_input(input: &ProjectionInput) -> EngineResult<()> {
    input.start_date.validate()?;
    input.end_date.validate()?;
    let horizon = input.start_date.days_until(input.end_date)?;
    if !(0..=MAX_PROJECTION_DAYS).contains(&horizon) {
        return Err(EngineError::InvalidInput(
            "projection horizon is outside the supported range",
        ));
    }
    if input.safety_floor_minor < 0 {
        return Err(EngineError::InvalidInput(
            "projection safety floor cannot be negative",
        ));
    }
    let event_limit =
        usize::try_from(input.max_events).map_err(|_| EngineError::ArithmeticOverflow)?;
    if event_limit == 0 || event_limit > MAX_PROJECTION_EVENTS {
        return Err(EngineError::LimitExceeded {
            resource: "projection events",
            limit: MAX_PROJECTION_EVENTS,
        });
    }
    validate_sources(&input.sources)
}

fn validate_sources(sources: &[ProjectionSource]) -> EngineResult<()> {
    if sources.len() > MAX_PROJECTION_SOURCES {
        return Err(EngineError::LimitExceeded {
            resource: "projection sources",
            limit: MAX_PROJECTION_SOURCES,
        });
    }
    let mut ids = HashSet::with_capacity(sources.len());
    for source in sources {
        source.first_date.validate()?;
        if let Some(end_date) = source.end_date {
            end_date.validate()?;
            if end_date < source.first_date {
                return Err(EngineError::InvalidInput(
                    "projection source ends before it starts",
                ));
            }
        }
        if source.source_id.is_empty()
            || source.source_id.len() > MAX_IDENTIFIER_BYTES
            || source.name.is_empty()
            || source.name.len() > MAX_NAME_BYTES
            || source.amount_minor == 0
        {
            return Err(EngineError::InvalidInput("projection source is invalid"));
        }
        if !ids.insert(source.source_id.as_str()) {
            return Err(EngineError::InvalidInput(
                "projection source IDs must be unique",
            ));
        }
    }
    Ok(())
}

fn expand_source(
    source: &ProjectionSource,
    projection_start: Date,
    projection_end: Date,
    event_limit: usize,
    output: &mut Vec<ExpandedEvent>,
) -> EngineResult<()> {
    let schedule_end = source
        .end_date
        .unwrap_or(projection_end)
        .min(projection_end);
    if schedule_end < projection_start || source.first_date > projection_end {
        return Ok(());
    }

    let mut occurrence = first_relevant_occurrence(source, projection_start)?;
    loop {
        let date = occurrence_date(source, occurrence)?;
        if date > schedule_end || date > projection_end {
            break;
        }
        if date >= projection_start {
            if output.len() >= event_limit {
                return Err(EngineError::LimitExceeded {
                    resource: "expanded projection events",
                    limit: event_limit,
                });
            }
            output.push(ExpandedEvent {
                source_id: source.source_id.clone(),
                occurrence,
                date,
                kind: source.kind,
                name: source.name.clone(),
                amount_minor: source.amount_minor,
            });
        }
        if source.frequency == Frequency::Once || date >= schedule_end {
            break;
        }
        occurrence = occurrence
            .checked_add(1)
            .ok_or(EngineError::ArithmeticOverflow)?;
    }
    Ok(())
}

fn first_relevant_occurrence(source: &ProjectionSource, start: Date) -> EngineResult<u32> {
    if source.first_date >= start || source.frequency == Frequency::Once {
        return Ok(0);
    }
    let days = source.first_date.days_until(start)?;
    let initial = match source.frequency {
        Frequency::Weekly => {
            u32::try_from(days / 7).map_err(|_| EngineError::ArithmeticOverflow)?
        }
        Frequency::Biweekly => {
            u32::try_from(days / 14).map_err(|_| EngineError::ArithmeticOverflow)?
        }
        Frequency::Monthly | Frequency::Quarterly | Frequency::Annually => {
            let month_difference = (start.year - source.first_date.year) * 12
                + i32::from(start.month)
                - i32::from(source.first_date.month);
            let step = match source.frequency {
                Frequency::Monthly => 1,
                Frequency::Quarterly => 3,
                Frequency::Annually => 12,
                _ => 1,
            };
            u32::try_from(month_difference.max(0) / step)
                .map_err(|_| EngineError::ArithmeticOverflow)?
        }
        Frequency::Once => 0,
    };
    let mut occurrence = initial;
    while occurrence_date(source, occurrence)? < start {
        occurrence = occurrence
            .checked_add(1)
            .ok_or(EngineError::ArithmeticOverflow)?;
    }
    Ok(occurrence)
}

fn occurrence_date(source: &ProjectionSource, occurrence: u32) -> EngineResult<Date> {
    match source.frequency {
        Frequency::Once => {
            if occurrence == 0 {
                Ok(source.first_date)
            } else {
                Err(EngineError::InvalidInput(
                    "one-time event has multiple occurrences",
                ))
            }
        }
        Frequency::Weekly => source.first_date.add_days(
            occurrence
                .checked_mul(7)
                .ok_or(EngineError::ArithmeticOverflow)?,
        ),
        Frequency::Biweekly => source.first_date.add_days(
            occurrence
                .checked_mul(14)
                .ok_or(EngineError::ArithmeticOverflow)?,
        ),
        Frequency::Monthly => source.first_date.add_months(occurrence),
        Frequency::Quarterly => source.first_date.add_months(
            occurrence
                .checked_mul(3)
                .ok_or(EngineError::ArithmeticOverflow)?,
        ),
        Frequency::Annually => source.first_date.add_months(
            occurrence
                .checked_mul(12)
                .ok_or(EngineError::ArithmeticOverflow)?,
        ),
    }
}
