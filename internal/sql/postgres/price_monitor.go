package postgres

// GetMonitoringPairs возвращает SQL запрос для получения пар мониторинга
const GetMonitoringPairs = `
			SELECT 
				stp.exchange_id,
				t.pair_id,
				e.name
			FROM
				(
				 SELECT
					 DISTINCT(msa.pair_id) AS pair_id
				 FROM
					 monitoring m
				 INNER JOIN 
					 monitoring_spot_arrays msa 
							ON m.id = msa.monitor_id 
				 WHERE
					 active = 1
				) t
			INNER JOIN 
				spot_trade_pair stp 
					ON t.pair_id = stp.id
			LEFT JOIN exchange e 
				ON e.id = stp.exchange_id
			WHERE 
				e.active = 1
			ORDER BY 
				exchange_id ASC`

// InsertPriceData возвращает SQL запрос для вставки данных о ценах
const InsertPriceData = `
			INSERT INTO price_spot_log (
				date,
				price_timestamp,
				pair_id,
				asks5_price, asks5_volume, asks4_price, asks4_volume, asks3_price, asks3_volume,
				asks2_price, asks2_volume, asks1_price, asks1_volume,
				bids1_price, bids1_volume, bids2_price, bids2_volume, bids3_price, bids3_volume,
				bids4_price, bids4_volume, bids5_price, bids5_volume
			) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)`
