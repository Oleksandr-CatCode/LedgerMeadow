#![forbid(unsafe_code)]

pub mod budget;
pub mod cashflow;
pub mod categorization;
pub mod date;
pub mod debt;
pub mod error;
pub mod forecast;
pub mod goals;
pub mod household;
pub mod insights;
pub mod money;
pub mod planning;
pub mod recurring;
pub mod rule;
pub mod space;
pub mod subscription;
pub mod transport;

pub use error::{EngineError, EngineResult};
