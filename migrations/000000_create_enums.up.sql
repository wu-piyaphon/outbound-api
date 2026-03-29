CREATE TYPE side AS ENUM('buy', 'sell');
CREATE TYPE order_status AS ENUM('pending', 'filled', 'rejected', 'cancelled');
CREATE TYPE transfer_type AS ENUM('deposit', 'withdrawal');