package orderbook

import "errors"

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

func (ob *OrderBook) bestLevel(side Side) (*OrderBookLevel, error) {
	if side == SideInvalid {
		return nil, errors.New("side is invalid")
	}
	if side == Bid {
		return ob.BestBidLevel, nil
	} else {
		return ob.BestAskLevel, nil
	}
}

func (ob *OrderBook) unlinkLevel(level *OrderBookLevel) {
	if level.ordersCnt > 0 {
		panic("INVALID_STATE: level is not empty")
	}

	if level.totalQty > 0 {
		panic("INVALID_STATE: level total qty is not zero")
	}

	// unlink level from book
	if level.prev != nil {
		level.prev.next = level.next
	}
	if level.next != nil {
		level.next.prev = level.prev
	}

	// update best stats
	if ob.BestBidLevel == level {
		ob.BestBidLevel = level.next
	}
	if ob.BestAskLevel == level {
		ob.BestAskLevel = level.next
	}

	// clear levels map
	if level.side == Bid {
		delete(ob.Bids, level.price)
	} else {
		delete(ob.Asks, level.price)
	}

}

func (ob *OrderBook) unlinkOrder(level *OrderBookLevel, order *OrderNode) {
	// unlink order from level
	if order.prev != nil {
		order.prev.next = order.next
	}
	if order.next != nil {
		order.next.prev = order.prev
	}

	// update level head and tail if needed
	if level.head == order {
		level.head = order.next
	}
	if level.tail == order {
		level.tail = order.prev
	}

	// update level stats
	level.totalQty -= order.Remaining
	level.ordersCnt--

	// update book stats
	if level.side == Bid {
		ob.TotalBidQty -= order.Remaining
	} else {
		ob.TotalAskQty -= order.Remaining
	}

	// unregister order
	delete(ob.Registry, order.ID)

	// if level is empty, remove it
	if level.ordersCnt == 0 {
		ob.unlinkLevel(level) // checked ordersCnt == 0
	}
}

func (ob *OrderBook) getSideTotalQty(side Side) Qty {
	if side == Bid {
		return ob.TotalBidQty
	} else {
		return ob.TotalAskQty
	}
}

// --------------------- Internal Methods ---------------------

func (ob *OrderBook) match(params MatchParams) (MatchResult, error) {
	if err := validateMatchParams(params); err != nil {
		return MatchResult{}, err
	}

	oppositeSide := getOppositeSide(params.Side)
	remainingQty := params.Qty

	trades := make([]Trade, 0)
	status := StatusInvalid
	resting := false

	if params.Type == Market {
		// check TIF
		if params.TIF == FOK {
			if remainingQty > ob.getSideTotalQty(oppositeSide) {
				// order can not be filled => reject FOK
				return MatchResult{
					Trades:       make([]Trade, 0),
					RemainingQty: remainingQty,
					Status:       StatusInvalid,
					Resting:      false,
				}, errors.New("FOK order can not be filled")
			}
		}

		for remainingQty > 0 {
			bestLevel, err := ob.bestLevel(oppositeSide)
			if err != nil {
				return MatchResult{}, err
			}
			if bestLevel == nil {
				// consumed all available liquidity
				if remainingQty == params.Qty {
					status = Cancelled // no liquidity found
				} else {
					status = PartiallyFilled
				}

				return MatchResult{
					Trades:       trades,
					RemainingQty: remainingQty,
					Status:       status,
					Resting:      resting,
				}, nil
			}

			levelTrades := make([]Trade, 0, bestLevel.ordersCnt)
			headOrder := bestLevel.head
			if headOrder == nil {
				// best level must not be empty
				panic("INVALID_STATE: best level is empty")
			}

			for headOrder != nil {
				nextOrder := headOrder.next // copy next order link (headOrder can be removed in fillOrder)

				var trade Trade
				remainingQty, trade, err = fillOrder(ob, bestLevel, headOrder, params.OrderID, remainingQty)
				if err != nil {
					panic("INVALID_STATE: " + err.Error())
				}
				levelTrades = append(levelTrades, trade)

				if remainingQty == 0 {
					break
				}

				headOrder = nextOrder
			}
			trades = append(trades, levelTrades...) // add level trades to global trades
		}

		// remaining qty is zero => order is fully filled
		status = Filled
		// resting is false for Market orders (always)
	} else {
		// get all opposite side orders, which are crossable with current order

		level, err := ob.bestLevel(oppositeSide)
		if err != nil {
			return MatchResult{}, err
		}

		for remainingQty > 0 && level != nil {
			if !crosses(level.price, params.Price, oppositeSide) {
				// best level is not crossable => finish
				break
			}

			headOrder := level.head
			for remainingQty > 0 && headOrder != nil {
				// consume order amount
				tradeQty := min(headOrder.Remaining, remainingQty)
				remainingQty -= tradeQty

				headOrder = headOrder.next
			}

			level = level.next // move to next level
		}
	}

	return MatchResult{
		Trades:       trades,
		RemainingQty: remainingQty,
		Status:       status,
		Resting:      resting,
	}, nil
}

func (ob *OrderBook) allocTradeID() TradeID {
	ob.NextTradeID++
	return ob.NextTradeID
}

// ---------------- Utils ----------------

func validateMatchParams(params MatchParams) error {
	if params.Side == SideInvalid {
		return errors.New("side is invalid")
	}
	if params.Type == OrderTypeInvalid {
		return errors.New("type is invalid")
	}
	if params.TIF == TIFInvalid {
		return errors.New("tif is invalid")
	}
	if params.Type == Market && params.Price != 0 {
		return errors.New("price is invalid")
	}
	if params.Type == Limit && params.Price <= 0 {
		return errors.New("price is invalid")
	}
	if params.Qty <= 0 {
		return errors.New("qty is invalid")
	}

	// validate TIF for order type
	if params.Type == Market {
		if params.TIF == GTC {
			return errors.New("GTC is not allowed for market orders")
		}
	}

	return nil
}

func fillOrder(ob *OrderBook, level *OrderBookLevel, order *OrderNode, currentID OrderID, remainingQty Qty) (Qty, Trade, error) {
	tradeQty := min(order.Remaining, remainingQty)

	if tradeQty == 0 {
		return remainingQty, Trade{}, errors.New("no trade qty available")
	}

	// update order stats
	order.Remaining -= tradeQty

	// update level stats
	level.totalQty -= tradeQty

	// update book stats
	if level.side == Bid {
		ob.TotalBidQty -= tradeQty
	} else {
		ob.TotalAskQty -= tradeQty
	}

	trade := Trade{
		ID:        ob.allocTradeID(),
		MakerID:   order.ID,
		TakerID:   currentID,
		Price:     order.Price,
		Qty:       tradeQty,
		MakerSide: order.Side,
	}

	// if order is fully filled, remove it
	if order.Remaining == 0 {
		ob.unlinkOrder(level, order)
	}

	return remainingQty - tradeQty, trade, nil
}

func crosses(levelPrice, orderPrice Price, oppositeSide Side) bool {
	if oppositeSide == SideInvalid {
		panic("INVALID_STATE: side is invalid")
	}
	if oppositeSide == Bid {
		return levelPrice >= orderPrice
	} else {
		return levelPrice <= orderPrice
	}
}