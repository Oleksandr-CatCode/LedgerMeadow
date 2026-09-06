use std::fmt::{Display, Formatter};

pub type EngineResult<T> = Result<T, EngineError>;

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum EngineError {
    InvalidInput(&'static str),
    LimitExceeded {
        resource: &'static str,
        limit: usize,
    },
    ArithmeticOverflow,
    PaymentDoesNotAmortize,
}

impl Display for EngineError {
    fn fmt(&self, formatter: &mut Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::InvalidInput(message) => formatter.write_str(message),
            Self::LimitExceeded { resource, limit } => {
                write!(formatter, "{resource} exceeds the limit of {limit}")
            }
            Self::ArithmeticOverflow => formatter.write_str("financial arithmetic overflow"),
            Self::PaymentDoesNotAmortize => {
                formatter.write_str("payment does not amortize the loan within the allowed term")
            }
        }
    }
}

impl std::error::Error for EngineError {}
