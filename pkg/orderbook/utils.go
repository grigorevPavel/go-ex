package orderbook

func getOppositeSide(side Side) Side {
	if side == SideInvalid {
		return SideInvalid
	} else if side == Bid {
		return Ask
	} else {
		return Bid
	}
}