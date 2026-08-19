package orderbook

// --------------------- Public Methods ---------------------

func New() *OrderBook {
	return &OrderBook{
		Bids:         make(OrderBookLevels),
		Asks:         make(OrderBookLevels),
		BestBidLevel: nil,
		BestAskLevel: nil,
		TotalBidQty:  0,
		TotalAskQty:  0,
		Registry:     make(OrderRegistry),
	}
}

func (ob *OrderBook) PlaceOrder(params PlaceOrderParams) (PlaceOrderResult, error) {
	return PlaceOrderResult{}, nil
}

func (ob *OrderBook) CancelOrder(params CancelOrderParams) (CancelOrderResult, error) {
	return CancelOrderResult{}, nil
}

func (ob *OrderBook) ReplaceOrder(params ReplaceOrderParams) (ReplaceOrderResult, error) {
	return ReplaceOrderResult{}, nil
}

// --------------------- Internal Methods ---------------------

func (ob *OrderBook) match() (MatchResult, error) {
	return MatchResult{}, nil
}
