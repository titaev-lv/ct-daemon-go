package mysql

// GetMonitoringPairs возвращает SQL запрос для получения пар мониторинга
const GetMonitoringPairs = `
			SELECT 
				stp.EXCHANGE_ID,
				t.PAIR_ID,
				e.NAME
			FROM
				(
				 SELECT
					 DISTINCT(msa.PAIR_ID) AS PAIR_ID
				 FROM
					 MONITORING m
				 INNER JOIN 
					 MONITORING_SPOT_ARRAYS msa 
							ON m.ID = msa.MONITOR_ID 
				 WHERE
					 ACTIVE = 1
				) t
			INNER JOIN 
				SPOT_TRADE_PAIR stp 
					ON t.PAIR_ID = stp.ID
			LEFT JOIN EXCHANGE e 
				ON e.ID = stp.EXCHANGE_ID
			WHERE 
				e.ACTIVE = 1
			ORDER BY 
				EXCHANGE_ID ASC`

// InsertPriceData возвращает SQL запрос для вставки данных о ценах
const InsertPriceData = `
			INSERT INTO PRICE_SPOT_LOG (
				DATE,
				PRICE_TIMESTAMP,
				PAIR_ID,
				ASKS5_PRICE, ASKS5_VOLUME, ASKS4_PRICE, ASKS4_VOLUME, ASKS3_PRICE, ASKS3_VOLUME,
				ASKS2_PRICE, ASKS2_VOLUME, ASKS1_PRICE, ASKS1_VOLUME,
				BIDS1_PRICE, BIDS1_VOLUME, BIDS2_PRICE, BIDS2_VOLUME, BIDS3_PRICE, BIDS3_VOLUME,
				BIDS4_PRICE, BIDS4_VOLUME, BIDS5_PRICE, BIDS5_VOLUME
			) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
