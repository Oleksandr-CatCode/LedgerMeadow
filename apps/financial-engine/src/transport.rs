use crate::EngineError;
use crate::cashflow::{
    CashFlowComponents, CashFlowInput, CashFlowOutput, CashFlowSummary, calculate_cash_flow,
};
use crate::categorization::{
    AccountKind, AnalysisTransaction, Categorization, CategorizationSignal,
    CategorizeTransactionsInput, CategorizeTransactionsOutput, CategoryCandidate, CategoryType,
    categorize_transactions,
};
use crate::date::Date;
use crate::forecast::{
    Frequency, ProjectionInput, ProjectionKind, ProjectionOutput, ProjectionSource,
    ProjectionStatus, TimelineEvent, build_projection, pay_cycle_horizon,
};
use crate::insights::{
    AnalyticsBreakdownInput, AnalyticsBreakdownOutput, AnalyticsBucket, NetWorthInput,
    NetWorthOutput, Valuation, calculate_analytics_breakdown, calculate_net_worth,
};
use crate::money::Currency;
use crate::planning::{
    PlanningAllocation, PlanningItem, PlanningItemKind, PlanningSummaryInput,
    PlanningSummaryOutput, calculate_planning_summary,
};
use crate::recurring::{
    CategoryRecurrenceEvidence, DetectRecurringInput, DetectRecurringOutput, RecurringCandidate,
    RecurringEvidence, RecurringKind, detect_recurring,
};
use crate::space::{
    AvailableToSpendInput, AvailableToSpendOutput, PlannedAllocation, ProtectedItem, ProtectedKind,
    calculate_available_to_spend,
};
use crate::subscription::{
    SubscriptionSummaryInput, SubscriptionSummaryOutput, calculate_subscription_summary,
};
use tonic::{Request, Response, Status};

pub mod pb {
    tonic::include_proto!("ledgermeadow.financial_engine.v1");
}

#[derive(Debug, Default)]
pub struct FinancialEngineService;

#[tonic::async_trait]
impl pb::financial_engine_server::FinancialEngine for FinancialEngineService {
    async fn categorize_transactions(
        &self,
        request: Request<pb::CategorizeTransactionsRequest>,
    ) -> Result<Response<pb::CategorizeTransactionsResponse>, Status> {
        let request = request.into_inner();
        let input = CategorizeTransactionsInput {
            transactions: request
                .transactions
                .into_iter()
                .map(analysis_transaction)
                .collect::<Result<Vec<_>, _>>()?,
            categories: request
                .categories
                .into_iter()
                .map(category_candidate)
                .collect::<Result<Vec<_>, _>>()?,
            signals: request
                .signals
                .into_iter()
                .map(categorization_signal)
                .collect(),
        };
        let output = categorize_transactions(&input).map_err(engine_status)?;
        Ok(Response::new(categorization_response(output)))
    }

    async fn detect_recurring(
        &self,
        request: Request<pb::DetectRecurringRequest>,
    ) -> Result<Response<pb::DetectRecurringResponse>, Status> {
        let request = request.into_inner();
        let input = DetectRecurringInput {
            as_of_date: date(request.as_of_date)?,
            transactions: request
                .transactions
                .into_iter()
                .map(analysis_transaction)
                .collect::<Result<Vec<_>, _>>()?,
            category_evidence: request
                .category_evidence
                .into_iter()
                .map(|evidence| {
                    let kind_hint = match pb::RecurringKind::try_from(evidence.kind_hint) {
                        Ok(pb::RecurringKind::Unspecified) => None,
                        Ok(pb::RecurringKind::Bill) => Some(RecurringKind::Bill),
                        Ok(pb::RecurringKind::Subscription) => Some(RecurringKind::Subscription),
                        Ok(pb::RecurringKind::Income) | Err(_) => {
                            return Err(Status::invalid_argument(
                                "category recurrence kind hint is invalid",
                            ));
                        }
                    };
                    Ok(CategoryRecurrenceEvidence {
                        category_id: evidence.category_id,
                        subscription_count: evidence.subscription_count,
                        bill_count: evidence.bill_count,
                        confirmed_subscription_count: evidence.confirmed_subscription_count,
                        confirmed_bill_count: evidence.confirmed_bill_count,
                        kind_hint,
                    })
                })
                .collect::<Result<Vec<_>, Status>>()?,
            prevailing_frequency: match pb::Frequency::try_from(request.prevailing_frequency) {
                Ok(pb::Frequency::Unspecified) => None,
                _ => Some(recurring_frequency(request.prevailing_frequency)?),
            },
        };
        let output = detect_recurring(&input).map_err(engine_status)?;
        Ok(Response::new(recurring_response(output)))
    }

    async fn evaluate_rules(
        &self,
        _request: Request<pb::EvaluateRulesRequest>,
    ) -> Result<Response<pb::EvaluateRulesResponse>, Status> {
        Err(Status::unimplemented("rule transport is not configured"))
    }

    async fn calculate_budget_status(
        &self,
        _request: Request<pb::CalculateBudgetStatusRequest>,
    ) -> Result<Response<pb::CalculateBudgetStatusResponse>, Status> {
        Err(Status::unimplemented("budget transport is not configured"))
    }

    async fn calculate_available_to_spend(
        &self,
        request: Request<pb::CalculateAvailableToSpendRequest>,
    ) -> Result<Response<pb::CalculateAvailableToSpendResponse>, Status> {
        let request = request.into_inner();
        let input = AvailableToSpendInput {
            currency: currency(request.currency)?,
            liquid_balance_minor: request.liquid_balance_minor,
            eligible_expected_income_minor: request.eligible_expected_income_minor,
            protected_items: request
                .protected_items
                .into_iter()
                .map(protected_item)
                .collect::<Result<Vec<_>, _>>()?,
            planned_allocations: request
                .planned_allocations
                .into_iter()
                .map(|item| PlannedAllocation {
                    id: item.id,
                    label: item.label,
                    amount_minor: item.amount_minor,
                })
                .collect(),
            safety_buffer_minor: request.safety_buffer_minor,
            projection_status: projection_status(request.projection_status)?,
            minimum_projected_balance_minor: request.minimum_projected_balance_minor,
        };
        let output = calculate_available_to_spend(&input).map_err(engine_status)?;
        Ok(Response::new(available_response(output)))
    }

    async fn calculate_cash_flow(
        &self,
        request: Request<pb::CalculateCashFlowRequest>,
    ) -> Result<Response<pb::CalculateCashFlowResponse>, Status> {
        let request = request.into_inner();
        let input = CashFlowInput {
            currency: currency(request.currency)?,
            actual: cash_flow_components(request.actual)?,
            projected: cash_flow_components(request.projected)?,
        };
        let output = calculate_cash_flow(&input).map_err(engine_status)?;
        Ok(Response::new(cash_flow_response(output)))
    }

    async fn build_projection(
        &self,
        request: Request<pb::BuildProjectionRequest>,
    ) -> Result<Response<pb::BuildProjectionResponse>, Status> {
        let request = request.into_inner();
        let sources = request
            .sources
            .into_iter()
            .map(projection_source)
            .collect::<Result<Vec<_>, _>>()?;
        let start_date = date(request.start_date)?;
        let end_date = if request.pay_cycle_horizon {
            pay_cycle_horizon(start_date, &sources).map_err(engine_status)?
        } else {
            date(request.end_date)?
        };
        let input = ProjectionInput {
            currency: currency(request.currency)?,
            starting_balance_minor: request.starting_balance_minor,
            start_date,
            end_date,
            safety_floor_minor: request.safety_floor_minor,
            max_events: request.max_events,
            sources,
        };
        let output = build_projection(&input).map_err(engine_status)?;
        Ok(Response::new(projection_response(output)))
    }

    async fn calculate_goal(
        &self,
        _request: Request<pb::CalculateGoalRequest>,
    ) -> Result<Response<pb::CalculateGoalResponse>, Status> {
        Err(Status::unimplemented("goal transport is not configured"))
    }

    async fn calculate_loan_scenario(
        &self,
        _request: Request<pb::CalculateLoanScenarioRequest>,
    ) -> Result<Response<pb::CalculateLoanScenarioResponse>, Status> {
        Err(Status::unimplemented("loan transport is not configured"))
    }

    async fn calculate_household_settlement(
        &self,
        _request: Request<pb::CalculateHouseholdSettlementRequest>,
    ) -> Result<Response<pb::CalculateHouseholdSettlementResponse>, Status> {
        Err(Status::unimplemented(
            "household transport is not configured",
        ))
    }

    async fn calculate_analytics_breakdown(
        &self,
        request: Request<pb::CalculateAnalyticsBreakdownRequest>,
    ) -> Result<Response<pb::CalculateAnalyticsBreakdownResponse>, Status> {
        let request = request.into_inner();
        let input = AnalyticsBreakdownInput {
            currency: currency(request.currency)?,
            buckets: request
                .buckets
                .into_iter()
                .map(|bucket| AnalyticsBucket {
                    id: bucket.id,
                    label: bucket.label,
                    amount_minor: bucket.amount_minor,
                    item_count: bucket.item_count,
                })
                .collect(),
        };
        let output = calculate_analytics_breakdown(&input).map_err(engine_status)?;
        Ok(Response::new(analytics_response(output)))
    }

    async fn calculate_net_worth(
        &self,
        request: Request<pb::CalculateNetWorthRequest>,
    ) -> Result<Response<pb::CalculateNetWorthResponse>, Status> {
        let request = request.into_inner();
        let input = NetWorthInput {
            currency: currency(request.currency)?,
            assets: request.assets.into_iter().map(valuation).collect(),
            liabilities: request.liabilities.into_iter().map(valuation).collect(),
        };
        let output = calculate_net_worth(&input).map_err(engine_status)?;
        Ok(Response::new(net_worth_response(output)))
    }

    async fn calculate_subscription_summary(
        &self,
        request: Request<pb::CalculateSubscriptionSummaryRequest>,
    ) -> Result<Response<pb::CalculateSubscriptionSummaryResponse>, Status> {
        let request = request.into_inner();
        let output = calculate_subscription_summary(&SubscriptionSummaryInput {
            currency: currency(request.currency)?,
            expected_amount_minor: request.expected_amount_minor,
            frequency: recurring_frequency(request.frequency)?,
            transaction_amounts_minor: request.transaction_amounts_minor,
        })
        .map_err(engine_status)?;
        Ok(Response::new(subscription_summary_response(output)))
    }

    async fn calculate_planning_summary(
        &self,
        request: Request<pb::CalculatePlanningSummaryRequest>,
    ) -> Result<Response<pb::CalculatePlanningSummaryResponse>, Status> {
        let request = request.into_inner();
        let input = PlanningSummaryInput {
            currency: currency(request.currency)?,
            as_of_date: date(request.as_of_date)?,
            plan_start_date: date(request.plan_start_date)?,
            plan_end_date: date(request.plan_end_date)?,
            items: request
                .items
                .into_iter()
                .map(planning_item)
                .collect::<Result<Vec<_>, _>>()?,
            allocations: request
                .allocations
                .into_iter()
                .map(planning_allocation)
                .collect::<Result<Vec<_>, _>>()?,
        };
        let output = calculate_planning_summary(&input).map_err(engine_status)?;
        Ok(Response::new(planning_summary_response(output)))
    }
}

fn currency(value: i32) -> Result<Currency, Status> {
    match pb::Currency::try_from(value) {
        Ok(pb::Currency::Cad) => Ok(Currency::Cad),
        Ok(pb::Currency::Usd) => Ok(Currency::Usd),
        _ => Err(Status::invalid_argument("currency is required")),
    }
}

fn pb_currency(value: Currency) -> i32 {
    match value {
        Currency::Cad => pb::Currency::Cad as i32,
        Currency::Usd => pb::Currency::Usd as i32,
    }
}

fn recurring_frequency(value: i32) -> Result<Frequency, Status> {
    match pb::Frequency::try_from(value) {
        Ok(pb::Frequency::Weekly) => Ok(Frequency::Weekly),
        Ok(pb::Frequency::Biweekly) => Ok(Frequency::Biweekly),
        Ok(pb::Frequency::Monthly) => Ok(Frequency::Monthly),
        Ok(pb::Frequency::Quarterly) => Ok(Frequency::Quarterly),
        Ok(pb::Frequency::Annually) => Ok(Frequency::Annually),
        _ => Err(Status::invalid_argument(
            "subscription frequency must recur",
        )),
    }
}

fn planning_frequency(value: i32) -> Result<Frequency, Status> {
    match pb::Frequency::try_from(value) {
        Ok(pb::Frequency::Once) => Ok(Frequency::Once),
        Ok(pb::Frequency::Weekly) => Ok(Frequency::Weekly),
        Ok(pb::Frequency::Biweekly) => Ok(Frequency::Biweekly),
        Ok(pb::Frequency::Monthly) => Ok(Frequency::Monthly),
        Ok(pb::Frequency::Quarterly) => Ok(Frequency::Quarterly),
        Ok(pb::Frequency::Annually) => Ok(Frequency::Annually),
        _ => Err(Status::invalid_argument("planning frequency is required")),
    }
}

fn date(value: Option<pb::Date>) -> Result<Date, Status> {
    let value = value.ok_or_else(|| Status::invalid_argument("date is required"))?;
    let month = u8::try_from(value.month)
        .map_err(|_| Status::invalid_argument("date is outside the supported calendar"))?;
    let day = u8::try_from(value.day)
        .map_err(|_| Status::invalid_argument("date is outside the supported calendar"))?;
    Date::new(value.year, month, day).map_err(engine_status)
}

fn pb_date(value: Date) -> pb::Date {
    pb::Date {
        year: value.year,
        month: u32::from(value.month),
        day: u32::from(value.day),
    }
}

fn analysis_transaction(value: pb::AnalysisTransaction) -> Result<AnalysisTransaction, Status> {
    Ok(AnalysisTransaction {
        transaction_id: value.transaction_id,
        account_id: value.account_id,
        name: value.name,
        merchant_name: value.merchant_name,
        original_description: value.original_description,
        amount_minor: value.amount_minor,
        currency: currency(value.currency)?,
        date: date(value.date)?,
        provider_category_primary: value.provider_category_primary,
        provider_category_detailed: value.provider_category_detailed,
        account_kind: account_kind(value.account_kind)?,
        category_id: value.category_id,
        category_type: category_type(value.category_type)?,
    })
}

fn account_kind(value: i32) -> Result<AccountKind, Status> {
    match pb::AnalysisAccountKind::try_from(value) {
        Ok(pb::AnalysisAccountKind::Asset) => Ok(AccountKind::Asset),
        Ok(pb::AnalysisAccountKind::Liability) => Ok(AccountKind::Liability),
        _ => Err(Status::invalid_argument(
            "transaction account kind is invalid",
        )),
    }
}

fn category_type(value: i32) -> Result<Option<CategoryType>, Status> {
    match pb::CategoryType::try_from(value) {
        Ok(pb::CategoryType::Unspecified) => Ok(None),
        Ok(pb::CategoryType::Income) => Ok(Some(CategoryType::Income)),
        Ok(pb::CategoryType::Expense) => Ok(Some(CategoryType::Expense)),
        Ok(pb::CategoryType::Transfer) => Ok(Some(CategoryType::Transfer)),
        Err(_) => Err(Status::invalid_argument(
            "transaction category type is invalid",
        )),
    }
}

fn required_category_type(value: i32) -> Result<CategoryType, Status> {
    category_type(value)?.ok_or_else(|| Status::invalid_argument("category type is required"))
}

fn category_candidate(value: pb::CategoryCandidate) -> Result<CategoryCandidate, Status> {
    Ok(CategoryCandidate {
        category_id: value.category_id,
        name: value.name,
        category_type: required_category_type(value.category_type)?,
        parent_name: value.parent_name,
    })
}

fn categorization_signal(value: pb::CategorizationSignal) -> CategorizationSignal {
    CategorizationSignal {
        learning_key: value.learning_key,
        category_id: value.category_id,
        personal_observation_count: value.personal_observation_count,
        global_user_count: value.global_user_count,
        global_total_contributor_count: value.global_total_contributor_count,
    }
}

fn categorization_response(
    output: CategorizeTransactionsOutput,
) -> pb::CategorizeTransactionsResponse {
    pb::CategorizeTransactionsResponse {
        categorizations: output
            .categorizations
            .into_iter()
            .map(categorization)
            .collect(),
    }
}

fn categorization(value: Categorization) -> pb::Categorization {
    pb::Categorization {
        transaction_id: value.transaction_id,
        confidence_basis_points: value.confidence_basis_points,
        reason: value.reason,
        category_id: value.category_id.unwrap_or_default(),
        learning_key: value.learning_key,
    }
}

fn recurring_response(output: DetectRecurringOutput) -> pb::DetectRecurringResponse {
    pb::DetectRecurringResponse {
        candidates: output
            .candidates
            .into_iter()
            .map(recurring_candidate)
            .collect(),
    }
}

fn recurring_candidate(value: RecurringCandidate) -> pb::RecurringCandidate {
    pb::RecurringCandidate {
        detection_key: value.detection_key,
        name: value.name,
        kind: match value.kind {
            RecurringKind::Income => pb::RecurringKind::Income as i32,
            RecurringKind::Bill => pb::RecurringKind::Bill as i32,
            RecurringKind::Subscription => pb::RecurringKind::Subscription as i32,
        },
        currency: pb_currency(value.currency),
        frequency: match value.frequency {
            Frequency::Once => pb::Frequency::Once as i32,
            Frequency::Weekly => pb::Frequency::Weekly as i32,
            Frequency::Biweekly => pb::Frequency::Biweekly as i32,
            Frequency::Monthly => pb::Frequency::Monthly as i32,
            Frequency::Quarterly => pb::Frequency::Quarterly as i32,
            Frequency::Annually => pb::Frequency::Annually as i32,
        },
        expected_amount_minor: value.expected_amount_minor,
        next_expected_at: Some(pb_date(value.next_expected_at)),
        confidence_basis_points: value.confidence_basis_points,
        occurrence_count: value.occurrence_count,
        account_id: value.account_id,
        supporting_transaction_ids: value.supporting_transaction_ids,
        explanation: value.explanation,
        category_id: value.category_id,
        evidence: match value.evidence {
            RecurringEvidence::Cadence => pb::RecurringEvidence::Cadence as i32,
            RecurringEvidence::LearnedCategory => pb::RecurringEvidence::LearnedCategory as i32,
        },
        observed_amount_minor: value.observed_amount_minor,
        amount_observation_transaction_ids: value.amount_observation_transaction_ids,
    }
}

fn protected_item(value: pb::ProtectedItem) -> Result<ProtectedItem, Status> {
    let kind = match pb::ProtectedKind::try_from(value.kind) {
        Ok(pb::ProtectedKind::UpcomingBill) => ProtectedKind::UpcomingBill,
        Ok(pb::ProtectedKind::Subscription) => ProtectedKind::Subscription,
        Ok(pb::ProtectedKind::ProtectedSpace) => ProtectedKind::ProtectedSpace,
        Ok(pb::ProtectedKind::RequiredGoalContribution) => ProtectedKind::RequiredGoalContribution,
        _ => return Err(Status::invalid_argument("protected item kind is required")),
    };
    Ok(ProtectedItem {
        id: value.id,
        label: value.label,
        kind,
        amount_minor: value.amount_minor,
    })
}

fn available_response(output: AvailableToSpendOutput) -> pb::CalculateAvailableToSpendResponse {
    pb::CalculateAvailableToSpendResponse {
        currency: pb_currency(output.currency),
        available_minor: output.available_minor,
        total_minor: output.total_minor,
        protected_minor: output.protected_minor,
        planned_allocations_minor: output.planned_allocations_minor,
        breakdown: output
            .breakdown
            .into_iter()
            .map(|item| pb::AvailableComponent {
                id: item.id,
                label: item.label,
                kind: match item.kind {
                    crate::space::AvailableComponentKind::LiquidBalance => {
                        pb::AvailableComponentKind::LiquidBalance as i32
                    }
                    crate::space::AvailableComponentKind::ExpectedIncome => {
                        pb::AvailableComponentKind::ExpectedIncome as i32
                    }
                    crate::space::AvailableComponentKind::UpcomingBill => {
                        pb::AvailableComponentKind::UpcomingBill as i32
                    }
                    crate::space::AvailableComponentKind::Subscription => {
                        pb::AvailableComponentKind::Subscription as i32
                    }
                    crate::space::AvailableComponentKind::ProtectedSpace => {
                        pb::AvailableComponentKind::ProtectedSpace as i32
                    }
                    crate::space::AvailableComponentKind::RequiredGoalContribution => {
                        pb::AvailableComponentKind::RequiredGoalContribution as i32
                    }
                    crate::space::AvailableComponentKind::SafetyBuffer => {
                        pb::AvailableComponentKind::SafetyBuffer as i32
                    }
                    crate::space::AvailableComponentKind::PlannedAllocation => {
                        pb::AvailableComponentKind::PlannedAllocation as i32
                    }
                    crate::space::AvailableComponentKind::PayCycleReserve => {
                        pb::AvailableComponentKind::PayCycleReserve as i32
                    }
                },
                amount_minor: item.amount_minor,
                effect_minor: item.effect_minor,
            })
            .collect(),
        status: pb_projection_status(output.status),
    }
}

fn projection_status(value: i32) -> Result<ProjectionStatus, Status> {
    match pb::ProjectionStatus::try_from(value) {
        Ok(pb::ProjectionStatus::OnTrack) => Ok(ProjectionStatus::OnTrack),
        Ok(pb::ProjectionStatus::Watch) => Ok(ProjectionStatus::Watch),
        Ok(pb::ProjectionStatus::AtRisk) => Ok(ProjectionStatus::AtRisk),
        _ => Err(Status::invalid_argument("projection status is required")),
    }
}

fn pb_projection_status(value: ProjectionStatus) -> i32 {
    match value {
        ProjectionStatus::OnTrack => pb::ProjectionStatus::OnTrack as i32,
        ProjectionStatus::Watch => pb::ProjectionStatus::Watch as i32,
        ProjectionStatus::AtRisk => pb::ProjectionStatus::AtRisk as i32,
    }
}

fn cash_flow_components(
    value: Option<pb::CashFlowComponents>,
) -> Result<CashFlowComponents, Status> {
    let value =
        value.ok_or_else(|| Status::invalid_argument("cash-flow components are required"))?;
    Ok(CashFlowComponents {
        income_minor: value.income_minor,
        fixed_outflow_minor: value.fixed_outflow_minor,
        variable_outflow_minor: value.variable_outflow_minor,
        subscriptions_minor: value.subscriptions_minor,
        savings_minor: value.savings_minor,
    })
}

fn cash_flow_response(output: CashFlowOutput) -> pb::CalculateCashFlowResponse {
    pb::CalculateCashFlowResponse {
        currency: pb_currency(output.currency),
        actual: Some(cash_flow_summary(output.actual)),
        projected: Some(cash_flow_summary(output.projected)),
    }
}

fn cash_flow_summary(value: CashFlowSummary) -> pb::CashFlowSummary {
    pb::CashFlowSummary {
        income_minor: value.income_minor,
        fixed_outflow_minor: value.fixed_outflow_minor,
        variable_outflow_minor: value.variable_outflow_minor,
        subscriptions_minor: value.subscriptions_minor,
        savings_minor: value.savings_minor,
        total_outflow_minor: value.total_outflow_minor,
        net_minor: value.net_minor,
    }
}

fn projection_source(value: pb::ProjectionSource) -> Result<ProjectionSource, Status> {
    let kind = match pb::ProjectionKind::try_from(value.kind) {
        Ok(pb::ProjectionKind::Salary) => ProjectionKind::Salary,
        Ok(pb::ProjectionKind::Bill) => ProjectionKind::Bill,
        Ok(pb::ProjectionKind::Subscription) => ProjectionKind::Subscription,
        Ok(pb::ProjectionKind::LoanPayment) => ProjectionKind::LoanPayment,
        Ok(pb::ProjectionKind::PlannedTransfer) => ProjectionKind::PlannedTransfer,
        Ok(pb::ProjectionKind::GoalContribution) => ProjectionKind::GoalContribution,
        Ok(pb::ProjectionKind::Other) => ProjectionKind::Other,
        _ => return Err(Status::invalid_argument("projection kind is required")),
    };
    let frequency = match pb::Frequency::try_from(value.frequency) {
        Ok(pb::Frequency::Once) => Frequency::Once,
        Ok(pb::Frequency::Weekly) => Frequency::Weekly,
        Ok(pb::Frequency::Biweekly) => Frequency::Biweekly,
        Ok(pb::Frequency::Monthly) => Frequency::Monthly,
        Ok(pb::Frequency::Quarterly) => Frequency::Quarterly,
        Ok(pb::Frequency::Annually) => Frequency::Annually,
        _ => return Err(Status::invalid_argument("projection frequency is required")),
    };
    Ok(ProjectionSource {
        source_id: value.source_id,
        name: value.name,
        kind,
        amount_minor: value.amount_minor,
        first_date: date(value.first_date)?,
        frequency,
        end_date: value.end_date.map(|value| date(Some(value))).transpose()?,
    })
}

fn projection_response(output: ProjectionOutput) -> pb::BuildProjectionResponse {
    pb::BuildProjectionResponse {
        currency: pb_currency(output.currency),
        ending_balance_minor: output.ending_balance_minor,
        minimum_balance_minor: output.minimum_balance_minor,
        minimum_balance_date: Some(pb_date(output.minimum_balance_date)),
        status: pb_projection_status(output.status),
        events: output.events.into_iter().map(timeline_event).collect(),
    }
}

fn timeline_event(value: TimelineEvent) -> pb::TimelineEvent {
    pb::TimelineEvent {
        source_id: value.source_id,
        occurrence: value.occurrence,
        date: Some(pb_date(value.date)),
        kind: match value.kind {
            ProjectionKind::Salary => pb::ProjectionKind::Salary as i32,
            ProjectionKind::Bill => pb::ProjectionKind::Bill as i32,
            ProjectionKind::Subscription => pb::ProjectionKind::Subscription as i32,
            ProjectionKind::LoanPayment => pb::ProjectionKind::LoanPayment as i32,
            ProjectionKind::PlannedTransfer => pb::ProjectionKind::PlannedTransfer as i32,
            ProjectionKind::GoalContribution => pb::ProjectionKind::GoalContribution as i32,
            ProjectionKind::Other => pb::ProjectionKind::Other as i32,
        },
        name: value.name,
        amount_minor: value.amount_minor,
        balance_before_minor: value.balance_before_minor,
        balance_after_minor: value.balance_after_minor,
    }
}

fn valuation(value: pb::Valuation) -> Valuation {
    Valuation {
        id: value.id,
        label: value.label,
        amount_minor: value.amount_minor,
    }
}

fn analytics_response(output: AnalyticsBreakdownOutput) -> pb::CalculateAnalyticsBreakdownResponse {
    pb::CalculateAnalyticsBreakdownResponse {
        currency: pb_currency(output.currency),
        total_minor: output.total_minor,
        item_count: output.item_count,
        buckets: output
            .buckets
            .into_iter()
            .map(|bucket| pb::AnalyticsBreakdownItem {
                id: bucket.id,
                label: bucket.label,
                amount_minor: bucket.amount_minor,
                item_count: bucket.item_count,
                share_basis_points: bucket.share_basis_points,
            })
            .collect(),
    }
}

fn net_worth_response(output: NetWorthOutput) -> pb::CalculateNetWorthResponse {
    pb::CalculateNetWorthResponse {
        currency: pb_currency(output.currency),
        total_assets_minor: output.total_assets_minor,
        total_liabilities_minor: output.total_liabilities_minor,
        net_worth_minor: output.net_worth_minor,
    }
}

fn subscription_summary_response(
    output: SubscriptionSummaryOutput,
) -> pb::CalculateSubscriptionSummaryResponse {
    pb::CalculateSubscriptionSummaryResponse {
        currency: pb_currency(output.currency),
        annual_cost_minor: output.annual_cost_minor,
        paid_this_year_minor: output.paid_this_year_minor,
        payment_amounts_minor: output.payment_amounts_minor,
    }
}

fn planning_item(value: pb::PlanningItem) -> Result<PlanningItem, Status> {
    let kind = match pb::PlanningItemKind::try_from(value.kind) {
        Ok(pb::PlanningItemKind::Income) => PlanningItemKind::Income,
        Ok(pb::PlanningItemKind::Bill) => PlanningItemKind::Bill,
        Ok(pb::PlanningItemKind::Subscription) => PlanningItemKind::Subscription,
        _ => return Err(Status::invalid_argument("planning item kind is required")),
    };
    Ok(PlanningItem {
        id: value.id,
        name: value.name,
        kind,
        expected_amount_minor: value.expected_amount_minor,
        next_date: date(value.next_date)?,
        frequency: planning_frequency(value.frequency)?,
    })
}

fn planning_allocation(value: pb::PlanningAllocation) -> Result<PlanningAllocation, Status> {
    Ok(PlanningAllocation {
        id: value.id,
        name: value.name,
        amount_minor: value.amount_minor,
        frequency: planning_frequency(value.frequency)?,
    })
}

fn planning_summary_response(
    output: PlanningSummaryOutput,
) -> pb::CalculatePlanningSummaryResponse {
    pb::CalculatePlanningSummaryResponse {
        currency: pb_currency(output.currency),
        expected_income_minor: output.expected_income_minor,
        committed_bills_minor: output.committed_bills_minor,
        committed_subscriptions_minor: output.committed_subscriptions_minor,
        planned_minor: output.planned_minor,
        unallocated_minor: output.unallocated_minor,
        recurring_bills_monthly_minor: output.recurring_bills_monthly_minor,
        recurring_subscriptions_monthly_minor: output.recurring_subscriptions_monthly_minor,
        recurring_monthly_minor: output.recurring_monthly_minor,
        recurring_share_percent: output.recurring_share_percent,
        next_7_days_minor: output.next_7_days_minor,
        next_30_days_minor: output.next_30_days_minor,
        annual_cost_minor: output.annual_cost_minor,
        allocations: output
            .allocations
            .into_iter()
            .map(|allocation| pb::PlanningAllocationAmount {
                id: allocation.id,
                name: allocation.name,
                amount_minor: allocation.amount_minor,
            })
            .collect(),
    }
}

fn engine_status(error: EngineError) -> Status {
    match error {
        EngineError::InvalidInput(message) => Status::invalid_argument(message),
        EngineError::LimitExceeded { .. } => Status::resource_exhausted(error.to_string()),
        EngineError::ArithmeticOverflow => Status::out_of_range(error.to_string()),
        EngineError::PaymentDoesNotAmortize => Status::invalid_argument(error.to_string()),
    }
}
